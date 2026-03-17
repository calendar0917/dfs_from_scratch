package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go-dfs/api"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// setupVolumeServer 创建测试用的 VolumeServer
func setupVolumeServer(t *testing.T) (*VolumeServer, string, func()) {
	tmpDir := t.TempDir()
	storageDir := filepath.Join(tmpDir, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatalf("创建存储目录失败: %v", err)
	}

	s := &VolumeServer{
		StorageDir: storageDir,
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return s, storageDir, cleanup
}

// mockUploadStream 模拟上传流
type mockUploadStream struct {
	grpc.ServerStream
	requests  []*api.UploadRequest
	response  *api.UploadResponse
	sendIndex int
	recvIndex int
}

func (m *mockUploadStream) SendAndClose(resp *api.UploadResponse) error {
	m.response = resp
	return nil
}

func (m *mockUploadStream) Recv() (*api.UploadRequest, error) {
	if m.recvIndex >= len(m.requests) {
		return nil, io.EOF
	}
	req := m.requests[m.recvIndex]
	m.recvIndex++
	return req, nil
}

func (m *mockUploadStream) Send(req *api.UploadRequest) error {
	return nil
}

func (m *mockUploadStream) Context() context.Context {
	return context.Background()
}

// mockDownloadStream 模拟下载流
type mockDownloadStream struct {
	grpc.ServerStream
	responses []*api.DownloadResponse
}

func (m *mockDownloadStream) Send(resp *api.DownloadResponse) error {
	// 拷贝一份，避免复用 buffer 导致内容被覆盖
	contentCopy := append([]byte(nil), resp.Content...)
	m.responses = append(m.responses, &api.DownloadResponse{Content: contentCopy})
	return nil
}

func (m *mockDownloadStream) Context() context.Context {
	return context.Background()
}

// ==================== UploadFile 测试 ====================

func TestUploadFile_Normal(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 构造上传请求序列
	requests := []*api.UploadRequest{
		{
			Data: &api.UploadRequest_Metadata{
				Metadata: &api.Metadata{
					Filename:    "test.txt",
					NextTargets: []string{},
				},
			},
		},
		{
			Data: &api.UploadRequest_Chunk{
				Chunk: []byte("Hello, World!"),
			},
		},
	}

	stream := &mockUploadStream{requests: requests}
	err := s.UploadFile(stream)
	
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	
	if stream.response == nil || !stream.response.Success {
		t.Error("应返回成功响应")
	}
	
	// 验证文件已写入
	content, err := os.ReadFile(filepath.Join(storageDir, "test.txt"))
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if string(content) != "Hello, World!" {
		t.Errorf("文件内容错误: 期望 'Hello, World!', 实际 '%s'", string(content))
	}
}

func TestUploadFile_MultipleChunks(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 多个数据块
	requests := []*api.UploadRequest{
		{
			Data: &api.UploadRequest_Metadata{
				Metadata: &api.Metadata{
					Filename:    "multi.txt",
					NextTargets: []string{},
				},
			},
		},
		{Data: &api.UploadRequest_Chunk{Chunk: []byte("Chunk1")}},
		{Data: &api.UploadRequest_Chunk{Chunk: []byte("Chunk2")}},
		{Data: &api.UploadRequest_Chunk{Chunk: []byte("Chunk3")}},
	}

	stream := &mockUploadStream{requests: requests}
	err := s.UploadFile(stream)
	
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	
	content, _ := os.ReadFile(filepath.Join(storageDir, "multi.txt"))
	if string(content) != "Chunk1Chunk2Chunk3" {
		t.Errorf("文件内容错误: %s", string(content))
	}
}

func TestUploadFile_EmptyFilename(t *testing.T) {
	s, _, cleanup := setupVolumeServer(t)
	defer cleanup()

	requests := []*api.UploadRequest{
		{
			Data: &api.UploadRequest_Metadata{
				Metadata: &api.Metadata{
					Filename:    "",
					NextTargets: []string{},
				},
			},
		},
	}

	stream := &mockUploadStream{requests: requests}
	err := s.UploadFile(stream)
	
	if err == nil {
		t.Error("空文件名应返回错误")
	}
	
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("错误码错误: 期望 %v, 实际 %v", codes.InvalidArgument, st.Code())
	}
}

func TestUploadFile_ChunkBeforeMetadata(t *testing.T) {
	s, _, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 先发送数据块，再发送元数据（错误顺序）
	requests := []*api.UploadRequest{
		{
			Data: &api.UploadRequest_Chunk{
				Chunk: []byte("data"),
			},
		},
	}

	stream := &mockUploadStream{requests: requests}
	err := s.UploadFile(stream)
	
	if err == nil {
		t.Error("应先收到元数据")
	}
	
	st, _ := status.FromError(err)
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("错误码错误: 期望 %v, 实际 %v", codes.FailedPrecondition, st.Code())
	}
}

func TestUploadFile_LargeFile(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 构造大文件（1MB）
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	requests := []*api.UploadRequest{
		{
			Data: &api.UploadRequest_Metadata{
				Metadata: &api.Metadata{
					Filename:    "large.bin",
					NextTargets: []string{},
				},
			},
		},
		{Data: &api.UploadRequest_Chunk{Chunk: largeData}},
	}

	stream := &mockUploadStream{requests: requests}
	err := s.UploadFile(stream)
	
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	
	// 验证文件大小
	info, err := os.Stat(filepath.Join(storageDir, "large.bin"))
	if err != nil {
		t.Fatalf("获取文件信息失败: %v", err)
	}
	if info.Size() != 1024*1024 {
		t.Errorf("文件大小错误: 期望 %d, 实际 %d", 1024*1024, info.Size())
	}
}

// ==================== DownloadFile 测试 ====================

func TestDownloadFile_Normal(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 先创建文件
	testContent := []byte("Download test content")
	testFile := filepath.Join(storageDir, "download.txt")
	if err := os.WriteFile(testFile, testContent, 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	stream := &mockDownloadStream{}
	err := s.DownloadFile(&api.DownloadRequest{Filename: "download.txt"}, stream)
	
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	
	// 重组接收到的数据
	var received []byte
	for _, resp := range stream.responses {
		received = append(received, resp.Content...)
	}
	
	if string(received) != string(testContent) {
		t.Errorf("下载内容错误: 期望 '%s', 实际 '%s'", string(testContent), string(received))
	}
}

func TestDownloadFile_NotFound(t *testing.T) {
	s, _, cleanup := setupVolumeServer(t)
	defer cleanup()

	stream := &mockDownloadStream{}
	err := s.DownloadFile(&api.DownloadRequest{Filename: "notexist.txt"}, stream)
	
	if err == nil {
		t.Error("不存在的文件应返回错误")
	}
	
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("错误码错误: 期望 %v, 实际 %v", codes.NotFound, st.Code())
	}
}

func TestDownloadFile_EmptyFilename(t *testing.T) {
	s, _, cleanup := setupVolumeServer(t)
	defer cleanup()

	stream := &mockDownloadStream{}
	err := s.DownloadFile(&api.DownloadRequest{Filename: ""}, stream)
	
	if err == nil {
		t.Error("空文件名应返回错误")
	}
	
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("错误码错误: 期望 %v, 实际 %v", codes.InvalidArgument, st.Code())
	}
}

// ==================== 工具函数测试 ====================

func TestVolumeServer_FileExists(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 创建文件
	os.WriteFile(filepath.Join(storageDir, "exists.txt"), []byte("test"), 0644)

	t.Run("文件存在", func(t *testing.T) {
		if !s.FileExists("exists.txt") {
			t.Error("应返回 true")
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		if s.FileExists("notexist.txt") {
			t.Error("应返回 false")
		}
	})
}

func TestVolumeServer_DeleteFile(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	// 创建文件
	testFile := filepath.Join(storageDir, "delete.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	t.Run("删除成功", func(t *testing.T) {
		err := s.DeleteFile("delete.txt")
		if err != nil {
			t.Errorf("删除失败: %v", err)
		}
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("文件应已被删除")
		}
	})

	t.Run("删除不存在的文件", func(t *testing.T) {
		err := s.DeleteFile("notexist.txt")
		if err == nil {
			t.Error("应返回错误")
		}
	})
}

func TestVolumeServer_GetFilePath(t *testing.T) {
	s, storageDir, cleanup := setupVolumeServer(t)
	defer cleanup()

	path := s.GetFilePath("test.txt")
	expected := filepath.Join(storageDir, "test.txt")
	if path != expected {
		t.Errorf("路径错误: 期望 %s, 实际 %s", expected, path)
	}
}
