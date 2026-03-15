package service

import (
	"context"
	"os"
	"path/filepath"

	"go-dfs/api"
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
	// 1. 准备存储路径
	// TODO 检查文件名，防止 ../ 这种路径穿越攻击
	savePath := filepath.Join(s.StorageDir, req.Filename)

	// 2. 写入文件（先用最简单的方案，不考虑大文件流式传输）
	err := os.WriteFile(savePath, req.Content, 0o644)
	if err != nil {
		return &api.UploadResponse{Success: false}, err
	}

	// 3. 返回成功响应
	return &api.UploadResponse{
		FileId:  req.Filename, // 暂时用文件名当 ID
		Success: true,
	}, nil
}
