package service

import (
	"context"
	"io"
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
func (s *VolumeServer) UploadFile(stream api.VolumeService_UploadFileServer) error {
	var nextStream api.VolumeService_UploadFileClient
	var file *os.File
	// 养成好习惯：函数退出时确保关闭文件
	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// 只有所有下游都确认成功，才返回成功
			if nextStream != nil {
				resp, err := nextStream.CloseAndRecv()
				if err != nil {
					return status.Errorf(codes.Internal, "下游确认失败: %v", err)
				}
				return stream.SendAndClose(resp)
			}
			if file != nil {
				file.Sync()  // 强行要求操作系统把数据写入磁盘介质
				file.Close() // 手动关闭，确保下载时文件已就绪
				file = nil   // 防止 defer 再次关闭
			}
			return stream.SendAndClose(&api.UploadResponse{Success: true})
		}
		if err != nil {
			return err
		}

		switch x := req.Data.(type) {
		case *api.UploadRequest_Metadata:
			// 1. 创建本地文件，严禁忽略 err
			path := filepath.Join(s.StorageDir, x.Metadata.Filename)
			file, err = os.Create(path)
			if err != nil {
				return status.Errorf(codes.Internal, "无法创建文件 %s: %v", path, err)
			}

			// 2. 转发逻辑
			if len(x.Metadata.NextTargets) > 1 {
				nextAddr := x.Metadata.NextTargets[1]
				// 修改名单给下一站
				x.Metadata.NextTargets = x.Metadata.NextTargets[1:]

				conn, err := grpc.NewClient(nextAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					return status.Errorf(codes.Internal, "无法连接下游 %s: %v", nextAddr, err)
				}
				// 注意：在实际项目中，conn 应该被缓存，这里 defer 会导致流还没完连接就断了
				// 但为了演示逻辑，先保持现状，或者去掉 defer

				client := api.NewVolumeServiceClient(conn)
				nextStream, err = client.UploadFile(context.Background())
				if err != nil {
					return status.Errorf(codes.Internal, "无法开启下游流: %v", err)
				}

				// 转发元数据
				if err := nextStream.Send(req); err != nil {
					return status.Errorf(codes.Internal, "转发元数据失败: %v", err)
				}
			}

		case *api.UploadRequest_Chunk:
			// 关键：检查状态！如果文件还没打开，说明协议顺序错了
			if file == nil {
				return status.Error(codes.FailedPrecondition, "收到数据块前未收到元数据")
			}

			// 写本地
			if _, err := file.Write(x.Chunk); err != nil {
				return status.Errorf(codes.Internal, "本地写入失败: %v", err)
			}

			// 转发
			if nextStream != nil {
				if err := nextStream.Send(req); err != nil {
					return status.Errorf(codes.Internal, "转发数据块失败: %v", err)
				}
			}
		}
	}
}

func (s *VolumeServer) DownloadFile(req *api.DownloadRequest, stream api.VolumeService_DownloadFileServer) error {
	// 打开文件
	path := filepath.Join(s.StorageDir, req.Filename)
	file, err := os.Open(path)
	if err != nil {
		return status.Errorf(codes.NotFound, "文件不存在： %v", err)
	}
	defer file.Close()
	// 循环读取并发送
	buffer := make([]byte, 64*1024) // 64 kb 一次发送
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "读取失败： %v", err)
		}
		if err := stream.Send(&api.DownloadResponse{
			Content: buffer[:n], // 发送给客户端
		}); err != nil {
			return err
		}
	}
	return nil
}
