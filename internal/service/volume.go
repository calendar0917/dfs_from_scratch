package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go-dfs/api"
	"go-dfs/internal/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	// 默认缓冲区大小 64KB
	defaultBufferSize = 64 * 1024

	defaultForwardPoolConnTimeout = 5 * time.Second
	defaultForwardStreamTimeout   = 5 * time.Second
)

// VolumeServer 实现 Volume 服务
type VolumeServer struct {
	api.UnimplementedVolumeServiceServer
	StorageDir string
	connPool   map[string]*pool.GRPCPool // 地址 -> 连接池
}

// NewVolumeServer 创建 VolumeServer
func NewVolumeServer(storageDir string) *VolumeServer {
	return &VolumeServer{
		StorageDir: storageDir,
		connPool:   make(map[string]*pool.GRPCPool),
	}
}

func newForwardPoolConfig() *pool.Config {
	return &pool.Config{
		InitialConn: 1,
		MaxConn:     5,
		MaxIdle:     3,
		ConnTimeout: defaultForwardPoolConnTimeout,
	}
}

// getPool 获取或创建连接池
func (s *VolumeServer) getPool(address string) (*pool.GRPCPool, error) {
	if p, ok := s.connPool[address]; ok {
		return p, nil
	}
	
	// 创建新连接池
	config := newForwardPoolConfig()
	
	p, err := pool.NewGRPCPool(address, config,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	
	s.connPool[address] = p
	return p, nil
}

// UploadFile 处理文件上传（支持链式复制）
func (s *VolumeServer) UploadFile(stream api.VolumeService_UploadFileServer) error {
	var nextStream api.VolumeService_UploadFileClient
	var file *os.File
	var filename string
	var pooledConn *pool.PooledConn

	defer func() {
		if file != nil {
			file.Close()
		}
		if pooledConn != nil {
			// 归还连接到池中
			pooledConn.Release()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return s.handleUploadComplete(stream, nextStream, file, filename)
		}
		if err != nil {
			return status.Errorf(codes.Internal, "接收数据失败: %v", err)
		}

		switch x := req.Data.(type) {
		case *api.UploadRequest_Metadata:
			pooledConn, err = s.handleMetadata(x.Metadata, &file, &nextStream)
			if err != nil {
				return err
			}
			filename = x.Metadata.Filename

		case *api.UploadRequest_Chunk:
			if err := s.handleChunk(x.Chunk, file, nextStream); err != nil {
				return err
			}
		}
	}
}

// handleMetadata 处理元数据
func (s *VolumeServer) handleMetadata(
	metadata *api.Metadata,
	file **os.File,
	nextStream *api.VolumeService_UploadFileClient,
) (*pool.PooledConn, error) {
	if metadata.Filename == "" {
		return nil, status.Error(codes.InvalidArgument, "文件名不能为空")
	}

	// 创建本地文件
	path := filepath.Join(s.StorageDir, metadata.Filename)
	if err := os.MkdirAll(s.StorageDir, 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "创建目录失败: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "无法创建文件 %s: %v", path, err)
	}
	*file = f

	// 转发到下一个节点
	if len(metadata.NextTargets) > 1 {
		nextAddr := metadata.NextTargets[1]
		metadata.NextTargets = metadata.NextTargets[1:]

		// 使用连接池
		p, err := s.getPool(nextAddr)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "无法获取连接池 %s: %v", nextAddr, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultForwardStreamTimeout)
		defer cancel()
		
		pooledConn, err := p.Get(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "无法获取连接 %s: %v", nextAddr, err)
		}

		client := api.NewVolumeServiceClient(pooledConn.ClientConn)
		ns, err := client.UploadFile(context.Background())
		if err != nil {
			pooledConn.Release()
			return nil, status.Errorf(codes.Internal, "无法开启下游流: %v", err)
		}
		*nextStream = ns

		if err := ns.Send(&api.UploadRequest{
			Data: &api.UploadRequest_Metadata{Metadata: metadata},
		}); err != nil {
			pooledConn.Release()
			return nil, status.Errorf(codes.Internal, "转发元数据失败: %v", err)
		}
		
		return pooledConn, nil
	}

	return nil, nil
}

// handleChunk 处理数据块
func (s *VolumeServer) handleChunk(
	chunk []byte,
	file *os.File,
	nextStream api.VolumeService_UploadFileClient,
) error {
	if file == nil {
		return status.Error(codes.FailedPrecondition, "收到数据块前未收到元数据")
	}

	if _, err := file.Write(chunk); err != nil {
		return status.Errorf(codes.Internal, "本地写入失败: %v", err)
	}

	if nextStream != nil {
		if err := nextStream.Send(&api.UploadRequest{
			Data: &api.UploadRequest_Chunk{Chunk: chunk},
		}); err != nil {
			return status.Errorf(codes.Internal, "转发数据块失败: %v", err)
		}
	}

	return nil
}

// handleUploadComplete 处理上传完成
func (s *VolumeServer) handleUploadComplete(
	stream api.VolumeService_UploadFileServer,
	nextStream api.VolumeService_UploadFileClient,
	file *os.File,
	filename string,
) error {
	// 同步文件到磁盘
	if file != nil {
		if err := file.Sync(); err != nil {
			return status.Errorf(codes.Internal, "文件同步失败: %v", err)
		}
		file.Close()
	}

	// 等待下游确认
	if nextStream != nil {
		resp, err := nextStream.CloseAndRecv()
		if err != nil {
			return status.Errorf(codes.Internal, "下游确认失败: %v", err)
		}
		if !resp.Success {
			return status.Error(codes.Internal, "下游返回失败")
		}
	}

	return stream.SendAndClose(&api.UploadResponse{
		Success: true,
		FileId:  filename,
	})
}

// DownloadFile 处理文件下载
func (s *VolumeServer) DownloadFile(
	req *api.DownloadRequest,
	stream api.VolumeService_DownloadFileServer,
) error {
	if req.Filename == "" {
		return status.Error(codes.InvalidArgument, "文件名不能为空")
	}

	path := filepath.Join(s.StorageDir, req.Filename)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return status.Errorf(codes.NotFound, "文件不存在: %s", req.Filename)
		}
		return status.Errorf(codes.Internal, "无法打开文件: %v", err)
	}
	defer file.Close()

	// 获取文件信息用于日志
	stat, err := file.Stat()
	if err != nil {
		return status.Errorf(codes.Internal, "获取文件信息失败: %v", err)
	}

	buffer := make([]byte, defaultBufferSize)
	sent := int64(0)

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "读取失败: %v", err)
		}

		if err := stream.Send(&api.DownloadResponse{Content: buffer[:n]}); err != nil {
			return status.Errorf(codes.Internal, "发送失败: %v", err)
		}
		sent += int64(n)
	}

	// 验证发送完整性
	if sent != stat.Size() {
		return status.Errorf(codes.Internal, "发送不完整: 期望 %d, 实际 %d", stat.Size(), sent)
	}

	return nil
}

// FileExists 检查文件是否存在
func (s *VolumeServer) FileExists(filename string) bool {
	path := filepath.Join(s.StorageDir, filename)
	_, err := os.Stat(path)
	return err == nil
}

// GetFilePath 获取文件完整路径
func (s *VolumeServer) GetFilePath(filename string) string {
	return filepath.Join(s.StorageDir, filename)
}

// DeleteFile 删除文件
func (s *VolumeServer) DeleteFile(filename string) error {
	path := filepath.Join(s.StorageDir, filename)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("文件不存在: %s", filename)
		}
		return fmt.Errorf("删除失败: %v", err)
	}
	return nil
}

// ClosePools 关闭所有连接池
func (s *VolumeServer) ClosePools() {
	for addr, p := range s.connPool {
		if err := p.Close(); err != nil {
			fmt.Printf("关闭连接池 %s 失败: %v\n", addr, err)
		}
	}
	s.connPool = make(map[string]*pool.GRPCPool)
}
