package bench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go-dfs/api"
	"go-dfs/internal/hash"
	"go-dfs/internal/service"
)

// ==================== Master 调度压测 ====================

func BenchmarkMaster_AssignVolume(b *testing.B) {
	s := service.NewMasterServer(":memory:")
	
	// 注册 5 个节点
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  fmt.Sprintf("node%d", i),
			Address: fmt.Sprintf("127.0.0.1:%d", 50052+i),
		})
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.AssignVolume(ctx, &api.AssignVolumeRequest{
				Filename: fmt.Sprintf("file%d.txt", i),
				FileSize: 1024 * 1024,
			})
			i++
		}
	})
}

func BenchmarkMaster_RegisterNode(b *testing.B) {
	s := service.NewMasterServer(":memory:")
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  fmt.Sprintf("node%d", i),
			Address: fmt.Sprintf("127.0.0.1:%d", 50052+i),
		})
	}
}

func BenchmarkMaster_GetFileLocation(b *testing.B) {
	s := service.NewMasterServer(":memory:")
	ctx := context.Background()
	
	// 注册节点并分配文件
	for i := 0; i < 5; i++ {
		s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  fmt.Sprintf("node%d", i),
			Address: fmt.Sprintf("127.0.0.1:%d", 50052+i),
		})
	}
	s.AssignVolume(ctx, &api.AssignVolumeRequest{
		Filename: "test.txt",
		FileSize: 1024,
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetFileLocation(ctx, &api.FileLocationRequest{
			Filename: "test.txt",
		})
	}
}

// ==================== 一致性哈希压测 ====================

func BenchmarkConsistentHash_Get(b *testing.B) {
	ch := hash.NewConsistentHash(150)
	ch.Add("node1", "node2", "node3", "node4", "node5")
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ch.Get(fmt.Sprintf("file%d.txt", i))
			i++
		}
	})
}

func BenchmarkConsistentHash_GetN(b *testing.B) {
	ch := hash.NewConsistentHash(150)
	ch.Add("node1", "node2", "node3", "node4", "node5")
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ch.GetN(fmt.Sprintf("file%d.txt", i), 3)
			i++
		}
	})
}

func BenchmarkConsistentHash_Add(b *testing.B) {
	ch := hash.NewConsistentHash(150)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Add(fmt.Sprintf("node%d", i))
	}
}

// ==================== Volume 读写压测 ====================

func BenchmarkVolume_WriteFile(b *testing.B) {
	tmpDir := b.TempDir()
	
	// 准备测试数据
	data := make([]byte, 64*1024) // 64KB
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	
	for i := 0; i < b.N; i++ {
		filename := fmt.Sprintf("bench%d.txt", i)
		path := filepath.Join(tmpDir, filename)
		os.WriteFile(path, data, 0644)
	}
}

func BenchmarkVolume_ReadFile(b *testing.B) {
	tmpDir := b.TempDir()
	
	// 准备测试文件
	testFile := filepath.Join(tmpDir, "test.txt")
	data := make([]byte, 1024*1024) // 1MB
	os.WriteFile(testFile, data, 0644)
	
	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	
	for i := 0; i < b.N; i++ {
		os.ReadFile(testFile)
	}
}

// ==================== 并发压测 ====================

func BenchmarkConcurrent_MultipleClients(b *testing.B) {
	s := service.NewMasterServer(":memory:")
	ctx := context.Background()
	
	// 注册 10 个节点
	for i := 0; i < 10; i++ {
		s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  fmt.Sprintf("node%d", i),
			Address: fmt.Sprintf("127.0.0.1:%d", 50052+i),
		})
	}
	
	b.ResetTimer()
	
	// 模拟 100 个并发客户端
	var wg sync.WaitGroup
	clients := 100
	requestsPerClient := b.N / clients
	
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			for i := 0; i < requestsPerClient; i++ {
				s.AssignVolume(ctx, &api.AssignVolumeRequest{
					Filename: fmt.Sprintf("client%d_file%d.txt", clientID, i),
					FileSize: 1024 * 1024,
				})
			}
		}(c)
	}
	
	wg.Wait()
}

// ==================== 内存分配压测 ====================

func BenchmarkMemory_HashRing(b *testing.B) {
	b.ReportAllocs()
	
	ch := hash.NewConsistentHash(150)
	ch.Add("node1", "node2", "node3")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Get(fmt.Sprintf("file%d.txt", i))
	}
}

func BenchmarkMemory_MasterAssign(b *testing.B) {
	b.ReportAllocs()
	
	s := service.NewMasterServer(":memory:")
	ctx := context.Background()
	
	for i := 0; i < 5; i++ {
		s.RegisterNode(ctx, &api.RegisterRequest{
			NodeId:  fmt.Sprintf("node%d", i),
			Address: fmt.Sprintf("127.0.0.1:%d", 50052+i),
		})
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.AssignVolume(ctx, &api.AssignVolumeRequest{
			Filename: fmt.Sprintf("file%d.txt", i),
			FileSize: 1024 * 1024,
		})
	}
}
