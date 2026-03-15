package service

import (
	"context"
	"log"
	"sync"
	"time"

	"go-dfs/api"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type NodeInfo struct {
	Address      string
	LastSeenTime time.Time
	nodeIndex    int
}

type MasterServer struct {
	api.UnimplementedMasterServiceServer
	// 实现 api.VolumeServiceServer 接口
	// 接受 RegisterRequest，返回 RegisterResponse
	mu       sync.RWMutex // 读写锁提高性能
	nodes    map[string]*NodeInfo
	nodeList []string // 快速轮询，只存 ID
	index    int      // 轮训序号
}

// NewMasterServer 是“构造函数”模式，确保 Map 被正确初始化
func NewMasterServer() *MasterServer {
	s := &MasterServer{
		nodes: make(map[string]*NodeInfo),
	}
	// 启动心跳检查协程
	go s.startHealthChecker()
	return s
}

func (s *MasterServer) startHealthChecker() {
	ticker := time.NewTicker(time.Second * 2)
	for range ticker.C {
		s.mu.Lock()
		// 先找出所有过期的 ID（不在这里动 Slice）
		var expiredIDs []string
		for id, info := range s.nodes {
			if time.Since(info.LastSeenTime) > 10*time.Second {
				expiredIDs = append(expiredIDs, id)
			}
		}

		// 统一清理
		for _, id := range expiredIDs {
			// 确认节点是否还在（防御式编程）
			info, ok := s.nodes[id]
			if !ok {
				continue
			}

			log.Printf("节点 %s 心跳超时，执行剔除", id)

			lastIdx := len(s.nodeList) - 1
			lastID := s.nodeList[lastIdx]

			// 获取当前节点在 slice 中的索引
			targetIdx := info.nodeIndex

			// 如果不是最后一个，才需要交换；如果是最后一个，直接缩减即可
			if targetIdx != lastIdx {
				s.nodeList[targetIdx] = lastID
				s.nodes[lastID].nodeIndex = targetIdx
			}

			// 缩容并从 Map 删除
			s.nodeList = s.nodeList[:lastIdx]
			delete(s.nodes, id)
		}

		s.mu.Unlock()
	}
}

func (s *MasterServer) RegisterNode(ctx context.Context, req *api.RegisterRequest) (*api.RegisterResponse, error) {
	// 参数校验
	if req.NodeId == "" || req.Address == "" {
		// 使用 gRPC 标准错误码返回，而不是让服务器崩溃
		return nil, status.Error(codes.InvalidArgument, "node_id 或 address 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.nodes[req.NodeId]
	// 更新或创建节点
	s.nodes[req.NodeId] = &NodeInfo{
		Address:      req.Address,
		LastSeenTime: time.Now(),
	}

	if !exists {
		// 注意：log.Printf 才支持格式化占位符，log.Println 不支持
		log.Printf("[新节点注册] ID=%s, 地址=%s", req.NodeId, req.Address)
		s.nodeList = append(s.nodeList, req.NodeId) // 注册时加入列表
		s.nodes[req.NodeId].nodeIndex = len(s.nodeList) - 1
	} else {
		// 是心跳包
		if info.Address != req.Address {
			log.Printf("[节点地址变更] ID: %s, %s -> %s", req.NodeId, info.Address, req.Address)
		}
		// 正常心跳不打印日志
	}
	return &api.RegisterResponse{Success: true}, nil
}

func (s *MasterServer) AssignVolume(ctx context.Context, req *api.AssignVolumeRequest) (*api.AssignVolumeResponse, error) {
	// 分配节点给 client
	s.mu.RLock() // 读锁
	defer s.mu.RUnlock()
	if len(s.nodes) == 0 {
		return nil, status.Error(codes.Unavailable, "没有可用的存储节点")
	}
	id := s.nodeList[s.index%len(s.nodeList)]
	addr := s.nodes[id].Address
	s.index++

	log.Printf("【Master】为文件 %s 分配了节点 %s", req.Filename, addr)
	return &api.AssignVolumeResponse{
		Address: addr,
		Token:   "todo-token",
	}, nil
}
