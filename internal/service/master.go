package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go-dfs/api"
	"go-dfs/internal/hash"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultReplicationFactor = 3
	heartbeatTimeout         = 10 * time.Second
	healthCheckInterval      = 2 * time.Second
	persistFileMode          = 0o644
)

// NodeInfo 存储节点信息
type NodeInfo struct {
	Address      string
	LastSeenTime time.Time
}

// MasterServer 实现 Master 服务
type MasterServer struct {
	api.UnimplementedMasterServiceServer

	nodes        sync.Map // map[string]*NodeInfo
	fileMetadata sync.Map // map[string][]string
	
	persistPath  string
	replication  int
	hashRing     *hash.ConsistentHash
	
	// 持久化控制
	persistMu     sync.Mutex
	persistDirty  bool
	persistStopCh chan struct{}
}

// NewMasterServer 创建 MasterServer 实例
func NewMasterServer(dbPath string) *MasterServer {
	s := &MasterServer{
		persistPath:   dbPath,
		replication:   defaultReplicationFactor,
		hashRing:      hash.NewConsistentHash(150),
		persistStopCh: make(chan struct{}),
	}
	
	// 从非内存路径加载数据
	if dbPath != ":memory:" {
		s.loadFromDisk()
	}
	
	go s.startHealthChecker()
	go s.startBackgroundPersister()
	return s
}

// SaveToDisk 持久化元数据到磁盘
func (s *MasterServer) SaveToDisk() error {
	if s.persistPath == ":memory:" {
		return nil
	}
	
	// 快速收集数据
	metadata := make(map[string][]string)
	s.fileMetadata.Range(func(key, value interface{}) bool {
		metadata[key.(string)] = value.([]string)
		return true
	})
	
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.persistPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpPath := s.persistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, persistFileMode); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.persistPath)
}

// loadFromDisk 从磁盘加载元数据
func (s *MasterServer) loadFromDisk() {
	data, err := os.ReadFile(s.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("未发现历史元数据，开启新账本")
			return
		}
		log.Printf("加载元数据失败: %v", err)
		return
	}

	var metadata map[string][]string
	if err := json.Unmarshal(data, &metadata); err != nil {
		log.Printf("解析元数据失败: %v", err)
		return
	}
	
	for k, v := range metadata {
		s.fileMetadata.Store(k, v)
	}
	log.Printf("已从磁盘恢复 %d 条文件元数据", len(metadata))
}

// startBackgroundPersister 后台定期持久化
func (s *MasterServer) startBackgroundPersister() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.persistIfDirty()
		case <-s.persistStopCh:
			return
		}
	}
}

// persistIfDirty 如果有变更则持久化
func (s *MasterServer) persistIfDirty() {
	s.persistMu.Lock()
	if !s.persistDirty {
		s.persistMu.Unlock()
		return
	}
	s.persistDirty = false
	s.persistMu.Unlock()
	
	if err := s.SaveToDisk(); err != nil {
		log.Printf("元数据持久化失败: %v", err)
	}
}

// markDirty 标记需要持久化
func (s *MasterServer) markDirty() {
	s.persistMu.Lock()
	s.persistDirty = true
	s.persistMu.Unlock()
}

// startHealthChecker 启动心跳检查协程
func (s *MasterServer) startHealthChecker() {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.checkNodeHealth()
	}
}

// checkNodeHealth 检查节点健康状态
func (s *MasterServer) checkNodeHealth() {
	now := time.Now()
	var expiredIDs []string
	
	s.nodes.Range(func(key, value interface{}) bool {
		id := key.(string)
		info := value.(*NodeInfo)
		if now.Sub(info.LastSeenTime) > heartbeatTimeout {
			expiredIDs = append(expiredIDs, id)
		}
		return true
	})

	for _, id := range expiredIDs {
		s.removeNode(id)
	}
}

// removeNode 从集群中移除节点
func (s *MasterServer) removeNode(id string) {
	value, ok := s.nodes.Load(id)
	if !ok {
		return
	}
	
	info := value.(*NodeInfo)
	log.Printf("节点 %s 心跳超时，执行剔除", id)
	
	s.hashRing.Remove(info.Address)
	s.nodes.Delete(id)
	s.markDirty()
}

// RegisterNode 注册节点或处理心跳
func (s *MasterServer) RegisterNode(ctx context.Context, req *api.RegisterRequest) (*api.RegisterResponse, error) {
	if req.NodeId == "" || req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id 和 address 不能为空")
	}

	value, exists := s.nodes.Load(req.NodeId)
	
	if !exists {
		log.Printf("[新节点注册] ID=%s, 地址=%s", req.NodeId, req.Address)
		s.nodes.Store(req.NodeId, &NodeInfo{
			Address:      req.Address,
			LastSeenTime: time.Now(),
		})
		s.hashRing.Add(req.Address)
		s.markDirty()
	} else {
		info := value.(*NodeInfo)
		if info.Address != req.Address {
			log.Printf("[节点地址变更] ID: %s, %s -> %s", req.NodeId, info.Address, req.Address)
			s.hashRing.Remove(info.Address)
			s.hashRing.Add(req.Address)
			info.Address = req.Address
			s.markDirty()
		}
		info.LastSeenTime = time.Now()
	}

	return &api.RegisterResponse{Success: true}, nil
}

// AssignVolume 分配存储节点
func (s *MasterServer) AssignVolume(ctx context.Context, req *api.AssignVolumeRequest) (*api.AssignVolumeResponse, error) {
	if req.Filename == "" {
		return nil, status.Error(codes.InvalidArgument, "文件名不能为空")
	}

	nodeCount := s.hashRing.NodeCount()
	if nodeCount == 0 {
		return nil, status.Error(codes.Unavailable, "没有可用的存储节点")
	}

	replicationFactor := s.replication
	if nodeCount < replicationFactor {
		return nil, status.Errorf(codes.ResourceExhausted,
			"可用节点不足(仅有 %d 个)，无法满足 %d 副本要求", nodeCount, replicationFactor)
	}

	pickedAddresses := s.hashRing.GetN(req.Filename, replicationFactor)
	if len(pickedAddresses) == 0 {
		return nil, status.Error(codes.Unavailable, "无法分配存储节点")
	}

	s.fileMetadata.Store(req.Filename, pickedAddresses)
	log.Printf("[调度] 文件 %s 分配链路: %v", req.Filename, pickedAddresses)
	s.markDirty()

	return &api.AssignVolumeResponse{
		Address: pickedAddresses,
		Token:   generateToken(),
	}, nil
}

// GetFileLocation 获取文件存储位置
func (s *MasterServer) GetFileLocation(ctx context.Context, req *api.FileLocationRequest) (*api.FileLocationResponse, error) {
	if req.Filename == "" {
		return nil, status.Error(codes.InvalidArgument, "文件名不能为空")
	}

	value, ok := s.fileMetadata.Load(req.Filename)
	if !ok {
		return nil, status.Error(codes.NotFound, "文件不存在")
	}

	addrs := value.([]string)
	if len(addrs) == 0 {
		return nil, status.Error(codes.NotFound, "文件不存在")
	}

	return &api.FileLocationResponse{Address: addrs[0]}, nil
}

// GetNodeCount 返回当前节点数（用于测试）
func (s *MasterServer) GetNodeCount() int {
	return s.hashRing.NodeCount()
}

// generateToken 生成简单令牌
func generateToken() string {
	return "token-" + time.Now().Format("20060102150405")
}

// ErrNodeNotFound 节点不存在错误
var ErrNodeNotFound = errors.New("节点不存在")
