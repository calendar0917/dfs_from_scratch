package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go-dfs/api"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// 默认副本数
	defaultReplicationFactor = 3
	// 心跳超时时间
	heartbeatTimeout = 10 * time.Second
	// 健康检查间隔
	healthCheckInterval = 2 * time.Second
	// 元数据持久化文件权限
	persistFileMode = 0o644
)

// NodeInfo 存储节点信息
type NodeInfo struct {
	Address      string
	LastSeenTime time.Time
	nodeIndex    int
}

// MasterServer 实现 Master 服务
type MasterServer struct {
	api.UnimplementedMasterServiceServer

	mu           sync.RWMutex
	nodes        map[string]*NodeInfo
	nodeList     []string
	index        int
	fileMetadata map[string][]string
	persistPath  string
	replication  int
}

// NewMasterServer 创建 MasterServer 实例
func NewMasterServer(dbPath string) *MasterServer {
	s := &MasterServer{
		nodes:        make(map[string]*NodeInfo),
		fileMetadata: make(map[string][]string),
		persistPath:  dbPath,
		replication:  defaultReplicationFactor,
	}
	s.loadFromDisk()
	go s.startHealthChecker()
	return s
}

// SaveToDisk 持久化元数据到磁盘
func (s *MasterServer) SaveToDisk() error {
	s.mu.RLock()
	data, err := json.Marshal(s.fileMetadata)
	s.mu.RUnlock()
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(s.persistPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// 原子写入：先写临时文件再重命名
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

	if err := json.Unmarshal(data, &s.fileMetadata); err != nil {
		log.Printf("解析元数据失败: %v", err)
		return
	}
	log.Printf("已从磁盘恢复 %d 条文件元数据", len(s.fileMetadata))
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
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var expiredIDs []string

	for id, info := range s.nodes {
		if now.Sub(info.LastSeenTime) > heartbeatTimeout {
			expiredIDs = append(expiredIDs, id)
		}
	}

	for _, id := range expiredIDs {
		s.removeNode(id)
	}
}

// removeNode 从集群中移除节点
func (s *MasterServer) removeNode(id string) {
	info, ok := s.nodes[id]
	if !ok {
		return
	}

	log.Printf("节点 %s 心跳超时，执行剔除", id)

	lastIdx := len(s.nodeList) - 1
	if lastIdx < 0 {
		return
	}

	targetIdx := info.nodeIndex
	lastID := s.nodeList[lastIdx]

	// 交换删除：将最后一个元素移到被删除位置
	if targetIdx != lastIdx {
		s.nodeList[targetIdx] = lastID
		if lastNode, ok := s.nodes[lastID]; ok {
			lastNode.nodeIndex = targetIdx
		}
	}

	s.nodeList = s.nodeList[:lastIdx]
	delete(s.nodes, id)
}

// RegisterNode 注册节点或处理心跳
func (s *MasterServer) RegisterNode(ctx context.Context, req *api.RegisterRequest) (*api.RegisterResponse, error) {
	if req.NodeId == "" || req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id 和 address 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.nodes[req.NodeId]
	if !exists {
		// 新节点注册
		log.Printf("[新节点注册] ID=%s, 地址=%s", req.NodeId, req.Address)
		s.nodeList = append(s.nodeList, req.NodeId)
		s.nodes[req.NodeId] = &NodeInfo{
			Address:      req.Address,
			LastSeenTime: time.Now(),
			nodeIndex:    len(s.nodeList) - 1,
		}
	} else {
		// 心跳更新
		if info.Address != req.Address {
			log.Printf("[节点地址变更] ID: %s, %s -> %s", req.NodeId, info.Address, req.Address)
			info.Address = req.Address
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

	s.mu.Lock()
	defer s.mu.Unlock()

	nodeCount := len(s.nodeList)
	if nodeCount == 0 {
		return nil, status.Error(codes.Unavailable, "没有可用的存储节点")
	}

	replicationFactor := s.replication
	if nodeCount < replicationFactor {
		return nil, status.Errorf(codes.ResourceExhausted,
			"可用节点不足(仅有 %d 个)，无法满足 %d 副本要求", nodeCount, replicationFactor)
	}

	// 轮询选择节点，确保副本分布在不同节点
	pickedAddresses := make([]string, 0, replicationFactor)
	seen := make(map[int]bool)

	for len(pickedAddresses) < replicationFactor {
		idx := s.index % nodeCount
		if seen[idx] {
			// 避免重复选择同一节点
			s.index++
			continue
		}
		seen[idx] = true
		id := s.nodeList[idx]
		pickedAddresses = append(pickedAddresses, s.nodes[id].Address)
		s.index++
	}

	// 记录元数据
	s.fileMetadata[req.Filename] = pickedAddresses
	log.Printf("[调度] 文件 %s 分配链路: %v", req.Filename, pickedAddresses)

	// 异步持久化
	go func() {
		if err := s.SaveToDisk(); err != nil {
			log.Printf("元数据持久化失败: %v", err)
		}
	}()

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

	s.mu.RLock()
	defer s.mu.RUnlock()

	addrs, ok := s.fileMetadata[req.Filename]
	if !ok || len(addrs) == 0 {
		return nil, status.Error(codes.NotFound, "文件不存在")
	}

	// 随机选择一个副本
	selectedAddr := addrs[rand.Intn(len(addrs))]
	return &api.FileLocationResponse{Address: selectedAddr}, nil
}

// GetNodeCount 返回当前节点数（用于测试）
func (s *MasterServer) GetNodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodeList)
}

// generateToken 生成简单令牌
func generateToken() string {
	return "token-" + time.Now().Format("20060102150405")
}

// ErrNodeNotFound 节点不存在错误
var ErrNodeNotFound = errors.New("节点不存在")
