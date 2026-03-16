package service

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"go-dfs/api"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// VolumeServer 实现 api.VolumeServiceServer 接口
type VolumeServer struct {
	// 必须嵌入这个结构体，这是 gRPC 为了保证向后兼容性的要求
	api.UnimplementedVolumeServiceServer

	// 存储路径：文件实际存放在硬盘的哪个目录？
	StorageDir string
}

// UploadFile 是在 .proto 里定义的接口的具体实现
func (s *VolumeServer) UploadFile(ctx context.Context, req *api.UploadRequest) (*api.UploadResponse, error) {
	// 1. 基础校验
	if req.Filename == "" {
		return nil, status.Error(codes.InvalidArgument, "文件名不能为空")
	}

	// 2. 本地持久化
	savePath := filepath.Join(s.StorageDir, req.Filename)
	if err := os.WriteFile(savePath, req.Content, 0o644); err != nil {
		return nil, status.Errorf(codes.Internal, "本地写入失败: %v", err)
	}
	log.Printf("【%s】本地写入成功: %s", s.StorageDir, req.Filename)

	// 3. 链式转发逻辑
	// 假设 req.NextTargets 包含 [V1, V2, V3]
	// 如果当前是 V1，则 NextTargets 应该是 [V2, V3]
	if len(req.NextTargets) > 1 {
		nextAddr := req.NextTargets[1]          // 取出下一个目标
		remainingTargets := req.NextTargets[1:] // 传递剩下的名单

		log.Printf("转发中: %s -> %s", req.Filename, nextAddr)

		// 建立连接（正式项目中这里应使用连接池）
		conn, err := grpc.NewClient(nextAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "连接下游节点 %s 失败: %v", nextAddr, err)
		}
		defer conn.Close()

		client := api.NewVolumeServiceClient(conn)

		// 注意：这里透传 ctx 的截止时间，保证整条链条超时时间一致
		resp, err := client.UploadFile(ctx, &api.UploadRequest{
			Filename:    req.Filename,
			Content:     req.Content,
			NextTargets: remainingTargets,
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "下游节点回传错误: %v", err)
		}

		if !resp.Success {
			return &api.UploadResponse{Success: false}, nil
		}
	}

	// 4. 全部成功（我是末尾节点，或者下游都写好了）
	return &api.UploadResponse{
		FileId:  req.Filename,
		Success: true,
	}, nil
}

func (s *VolumeServer) DownloadFile(ctx context.Context, req *api.DownloadRequest) (*api.DownloadResponse, error) {
	savePath := filepath.Join(s.StorageDir, req.Filename)

	// 读取本地文件
	content, err := os.ReadFile(savePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "文件不存在")
		}
		return nil, status.Errorf(codes.Internal, "读取失败: %v", err)
	}

	log.Printf("【%s】文件被下载: %s", s.StorageDir, req.Filename)
	return &api.DownloadResponse{Content: content}, nil
}
