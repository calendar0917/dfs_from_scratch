package service

import (
	"context"
	"log"
	"sync"

	"go-dfs/api"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MasterServer struct {
	api.UnimplementedMasterServiceServer
	// 实现 api.VolumeServiceServer 接口
	// 接受 RegisterRequest，返回 RegisterResponse
	mu    sync.RWMutex // 读写锁提高性能
	nodes map[string]string
}

// NewMasterServer 是“构造函数”模式，确保 Map 被正确初始化
func NewMasterServer() *MasterServer {
	return &MasterServer{
		nodes: make(map[string]string),
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

	// 记录节点
	s.nodes[req.NodeId] = req.Address

	// 注意：log.Printf 才支持格式化占位符，log.Println 不支持
	log.Printf("【Master】发现新节点成功: ID=%s, 地址=%s", req.NodeId, req.Address)

	return &api.RegisterResponse{Success: true}, nil
}

func (s *MasterServer) AssignVolume(ctx context.Context, req *api.AssignVolumeRequest) (*api.AssignVolumeResponse, error) {
	// 分配节点给 client
	s.mu.RLock() // 读锁
	defer s.mu.RUnlock()
	if len(s.nodes) == 0 {
		return nil, status.Error(codes.Unavailable, "没有可用的存储节点")
	}
	// 简单随机策略，map 的遍历本身是伪随机的
	var addr string
	for _, a := range s.nodes {
		addr = a
		break
	}
	log.Printf("【Master】为文件 %s 分配了节点 %s", req.Filename, addr)
	return &api.AssignVolumeResponse{
		Address: addr,
		Token:   "todo-token",
	}, nil
}
