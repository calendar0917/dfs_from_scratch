package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go-dfs/api"
)

// setupTestServer 创建测试用的 MasterServer
func setupTestServer(t *testing.T) (*MasterServer, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	s := NewMasterServer(dbPath)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return s, cleanup
}

// registerTestNode 注册测试节点
func registerTestNode(t *testing.T, s *MasterServer, nodeID, address string) {
	ctx := context.Background()
	_, err := s.RegisterNode(ctx, &api.RegisterRequest{
		NodeId:  nodeID,
		Address: address,
	})
	if err != nil {
		t.Fatalf("注册节点失败: %v", err)
	}
}

func TestNewMasterServer(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	if s == nil {
		t.Fatal("NewMasterServer 返回 nil")
	}
	if s.nodes == nil {
		t.Error("nodes map 未初始化")
	}
	if s.fileMetadata == nil {
		t.Error("fileMetadata map 未初始化")
	}
	if s.replication != defaultReplicationFactor {
		t.Errorf("默认副本数错误: 期望 %d, 实际 %d", defaultReplicationFactor, s.replication)
	}
}

func TestRegisterNode(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("新节点注册", func(t *testing.T) {
		resp, err := s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  "node1",
			Address: "127.0.0.1:50052",
		})
		if err != nil {
			t.Fatalf("注册失败: %v", err)
		}
		if !resp.Success {
			t.Error("注册应返回成功")
		}
		if s.GetNodeCount() != 1 {
			t.Errorf("节点数错误: 期望 1, 实际 %d", s.GetNodeCount())
		}
	})

	t.Run("重复注册（心跳）", func(t *testing.T) {
		resp, err := s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  "node1",
			Address: "127.0.0.1:50052",
		})
		if err != nil {
			t.Fatalf("心跳失败: %v", err)
		}
		if !resp.Success {
			t.Error("心跳应返回成功")
		}
		if s.GetNodeCount() != 1 {
			t.Errorf("节点数应保持 1, 实际 %d", s.GetNodeCount())
		}
	})

	t.Run("地址变更", func(t *testing.T) {
		resp, err := s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  "node1",
			Address: "127.0.0.1:50053",
		})
		if err != nil {
			t.Fatalf("地址变更失败: %v", err)
		}
		if !resp.Success {
			t.Error("地址变更应返回成功")
		}
	})

	t.Run("空参数", func(t *testing.T) {
		_, err := s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  "",
			Address: "127.0.0.1:50052",
		})
		if err == nil {
			t.Error("空 node_id 应返回错误")
		}

		_, err = s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  "node2",
			Address: "",
		})
		if err == nil {
			t.Error("空 address 应返回错误")
		}
	})

	t.Run("多个节点", func(t *testing.T) {
		for i := 2; i <= 5; i++ {
			registerTestNode(t, s, fmt.Sprintf("node%d", i), fmt.Sprintf("127.0.0.1:%d", 50051+i))
		}
		if s.GetNodeCount() != 5 {
			t.Errorf("节点数错误: 期望 5, 实际 %d", s.GetNodeCount())
		}
	})
}

func TestAssignVolume(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("空集群请求", func(t *testing.T) {
		_, err := s.AssignVolume(ctx, &api.AssignVolumeRequest{Filename: "test.txt"})
		if err == nil {
			t.Error("空集群应返回错误")
		}
	})

	t.Run("节点不足", func(t *testing.T) {
		// 只注册 2 个节点，但需要 3 个副本
		registerTestNode(t, s, "v1", "127.0.0.1:50052")
		registerTestNode(t, s, "v2", "127.0.0.1:50053")

		_, err := s.AssignVolume(ctx, &api.AssignVolumeRequest{Filename: "test.txt"})
		if err == nil {
			t.Error("节点不足应返回错误")
		}
	})

	t.Run("正常分配", func(t *testing.T) {
		// 注册 3 个节点
		registerTestNode(t, s, "v3", "127.0.0.1:50054")

		resp, err := s.AssignVolume(ctx, &api.AssignVolumeRequest{Filename: "test.txt"})
		if err != nil {
			t.Fatalf("分配失败: %v", err)
		}
		if len(resp.Address) != 3 {
			t.Errorf("应返回 3 个地址, 实际 %d", len(resp.Address))
		}
	})

	t.Run("轮询分配", func(t *testing.T) {
		// 注册更多节点
		for i := 4; i <= 6; i++ {
			registerTestNode(t, s, fmt.Sprintf("v%d", i), fmt.Sprintf("127.0.0.1:%d", 50051+i))
		}

		// 多次分配，检查是否轮询
		assignments := make(map[string]int)
		for i := 0; i < 10; i++ {
			resp, err := s.AssignVolume(ctx, &api.AssignVolumeRequest{
				Filename: fmt.Sprintf("file%d.txt", i),
			})
			if err != nil {
				t.Fatalf("分配失败: %v", err)
			}
			for _, addr := range resp.Address {
				assignments[addr]++
			}
		}

		// 检查负载是否分散
		for addr, count := range assignments {
			if count < 3 {
				t.Errorf("节点 %s 分配次数过少: %d", addr, count)
			}
		}
	})

	t.Run("空文件名", func(t *testing.T) {
		_, err := s.AssignVolume(ctx, &api.AssignVolumeRequest{Filename: ""})
		if err == nil {
			t.Error("空文件名应返回错误")
		}
	})
}

func TestGetFileLocation(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// 注册节点并分配文件
	for i := 1; i <= 3; i++ {
		registerTestNode(t, s, fmt.Sprintf("v%d", i), fmt.Sprintf("127.0.0.1:%d", 50051+i))
	}

	_, err := s.AssignVolume(ctx, &api.AssignVolumeRequest{Filename: "exists.txt"})
	if err != nil {
		t.Fatalf("分配失败: %v", err)
	}

	t.Run("文件存在", func(t *testing.T) {
		resp, err := s.GetFileLocation(ctx, &api.FileLocationRequest{Filename: "exists.txt"})
		if err != nil {
			t.Fatalf("获取位置失败: %v", err)
		}
		if resp.Address == "" {
			t.Error("应返回有效地址")
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		_, err := s.GetFileLocation(ctx, &api.FileLocationRequest{Filename: "notexists.txt"})
		if err == nil {
			t.Error("不存在的文件应返回错误")
		}
	})

	t.Run("空文件名", func(t *testing.T) {
		_, err := s.GetFileLocation(ctx, &api.FileLocationRequest{Filename: ""})
		if err == nil {
			t.Error("空文件名应返回错误")
		}
	})
}

func TestHealthChecker(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	// 注册节点
	registerTestNode(t, s, "h1", "127.0.0.1:50052")

	if s.GetNodeCount() != 1 {
		t.Fatalf("初始节点数错误: 期望 1, 实际 %d", s.GetNodeCount())
	}

	// 等待心跳超时
	time.Sleep(heartbeatTimeout + healthCheckInterval + time.Second)

	// 节点应该被移除
	if s.GetNodeCount() != 0 {
		t.Errorf("超时节点应被移除: 期望 0, 实际 %d", s.GetNodeCount())
	}
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 创建服务器并写入数据
	s1 := NewMasterServer(dbPath)
	for i := 1; i <= 3; i++ {
		registerTestNode(t, s1, fmt.Sprintf("v%d", i), fmt.Sprintf("127.0.0.1:%d", 50051+i))
	}

	ctx := context.Background()
	_, err := s1.AssignVolume(ctx, &api.AssignVolumeRequest{Filename: "persist.txt"})
	if err != nil {
		t.Fatalf("分配失败: %v", err)
	}

	// 等待异步持久化完成
	time.Sleep(100 * time.Millisecond)

	// 创建新服务器，应该能恢复数据
	s2 := NewMasterServer(dbPath)

	resp, err := s2.GetFileLocation(ctx, &api.FileLocationRequest{Filename: "persist.txt"})
	if err != nil {
		t.Fatalf("恢复后获取位置失败: %v", err)
	}
	if resp.Address == "" {
		t.Error("应返回有效地址")
	}
}
