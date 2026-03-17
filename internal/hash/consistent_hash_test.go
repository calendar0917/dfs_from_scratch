package hash

import (
	"fmt"
	"testing"
)

func TestNewConsistentHash(t *testing.T) {
	ch := NewConsistentHash(0)
	if ch.replicas != 150 {
		t.Errorf("默认虚拟节点数错误: 期望 150, 实际 %d", ch.replicas)
	}

	ch2 := NewConsistentHash(300)
	if ch2.replicas != 300 {
		t.Errorf("自定义虚拟节点数错误: 期望 300, 实际 %d", ch2.replicas)
	}
}

func TestConsistentHash_Add(t *testing.T) {
	ch := NewConsistentHash(10)

	ch.Add("node1")
	if ch.NodeCount() != 1 {
		t.Errorf("节点数错误: 期望 1, 实际 %d", ch.NodeCount())
	}
	if ch.VirtualCount() != 10 {
		t.Errorf("虚拟节点数错误: 期望 10, 实际 %d", ch.VirtualCount())
	}

	// 添加多个节点
	ch.Add("node2", "node3")
	if ch.NodeCount() != 3 {
		t.Errorf("节点数错误: 期望 3, 实际 %d", ch.NodeCount())
	}
	if ch.VirtualCount() != 30 {
		t.Errorf("虚拟节点数错误: 期望 30, 实际 %d", ch.VirtualCount())
	}
}

func TestConsistentHash_Get(t *testing.T) {
	ch := NewConsistentHash(100)
	ch.Add("node1", "node2", "node3")

	// 测试获取节点
	node := ch.Get("file1.txt")
	if node == "" {
		t.Error("应返回有效节点")
	}

	// 相同 key 应返回相同节点
	node2 := ch.Get("file1.txt")
	if node != node2 {
		t.Error("相同 key 应映射到相同节点")
	}

	// 空 key
	if ch.Get("") != "" {
		t.Error("空 key 应返回空")
	}

	// 空环
	emptyCh := NewConsistentHash(10)
	if emptyCh.Get("file.txt") != "" {
		t.Error("空环应返回空")
	}
}

func TestConsistentHash_GetN(t *testing.T) {
	ch := NewConsistentHash(100)
	ch.Add("node1", "node2", "node3", "node4", "node5")

	// 获取 3 个节点
	nodes := ch.GetN("file.txt", 3)
	if len(nodes) != 3 {
		t.Errorf("应返回 3 个节点, 实际 %d", len(nodes))
	}

	// 检查节点不重复
	seen := make(map[string]bool)
	for _, node := range nodes {
		if seen[node] {
			t.Errorf("节点 %s 重复", node)
		}
		seen[node] = true
	}

	// 请求超过实际节点数
	nodes = ch.GetN("file.txt", 10)
	if len(nodes) != 5 {
		t.Errorf("应返回所有节点(5个), 实际 %d", len(nodes))
	}

	// 无效参数
	if ch.GetN("", 3) != nil {
		t.Error("空 key 应返回 nil")
	}
	if ch.GetN("file.txt", 0) != nil {
		t.Error("n=0 应返回 nil")
	}
}

func TestConsistentHash_Remove(t *testing.T) {
	ch := NewConsistentHash(10)
	ch.Add("node1", "node2", "node3")

	// 移除前记录映射
	mappings := make(map[string]string)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("file%d.txt", i)
		mappings[key] = ch.Get(key)
	}

	// 移除 node2
	ch.Remove("node2")

	if ch.NodeCount() != 2 {
		t.Errorf("节点数错误: 期望 2, 实际 %d", ch.NodeCount())
	}

	// 检查原本映射到 node2 的 key 是否重新映射
	changed := 0
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("file%d.txt", i)
		newNode := ch.Get(key)
		if mappings[key] == "node2" && newNode != "node2" {
			changed++
		}
		// 不应再映射到 node2
		if newNode == "node2" {
			t.Errorf("key %s 不应再映射到 node2", key)
		}
	}

	// 只有原本映射到 node2 的 key 应该改变
	if changed == 0 {
		t.Error("应有部分 key 重新映射")
	}
}

func TestConsistentHash_Distribution(t *testing.T) {
	ch := NewConsistentHash(150)
	ch.Add("node1", "node2", "node3")

	// 统计 10000 个 key 的分布
	distribution := make(map[string]int)
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("file%d.txt", i)
		node := ch.Get(key)
		distribution[node]++
	}

	// 检查每个节点都有数据
	if len(distribution) != 3 {
		t.Errorf("应有 3 个节点有数据, 实际 %d", len(distribution))
	}

	// 检查负载均衡（每个节点应该在 25%-45% 之间，CRC32 不够均匀）
	for node, count := range distribution {
		ratio := float64(count) / 10000.0
		if ratio < 0.25 || ratio > 0.45 {
			t.Errorf("节点 %s 负载不均衡: %.2f%%", node, ratio*100)
		}
	}
}

func TestConsistentHash_MigrationRate(t *testing.T) {
	// 测试节点增删时的数据迁移率
	ch := NewConsistentHash(150)
	ch.Add("node1", "node2", "node3")

	// 记录 10000 个 key 的映射
	mappings := make(map[string]string)
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("file%d.txt", i)
		mappings[key] = ch.Get(key)
	}

	// 添加新节点
	ch.Add("node4")

	// 统计迁移率
	migrated := 0
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("file%d.txt", i)
		if ch.Get(key) != mappings[key] {
			migrated++
		}
	}

	migrationRate := float64(migrated) / 10000.0
	// 理想情况下，添加 1 个节点到 4 个节点，迁移率应该接近 25%
	if migrationRate < 0.20 || migrationRate > 0.30 {
		t.Errorf("数据迁移率异常: %.2f%% (期望约 25%%)", migrationRate*100)
	}

	t.Logf("添加节点后的数据迁移率: %.2f%%", migrationRate*100)
}

func TestConsistentHash_Concurrent(t *testing.T) {
	ch := NewConsistentHash(100)
	ch.Add("node1", "node2", "node3")

	// 并发读写
	done := make(chan bool, 10)

	// 5 个 goroutine 读
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 1000; j++ {
				key := fmt.Sprintf("goroutine%d_file%d.txt", id, j)
				ch.Get(key)
			}
			done <- true
		}(i)
	}

	// 5 个 goroutine 写
	for i := 0; i < 5; i++ {
		go func(id int) {
			node := fmt.Sprintf("new_node%d", id)
			ch.Add(node)
			ch.Remove(node)
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkConsistentHash_Get(b *testing.B) {
	ch := NewConsistentHash(150)
	ch.Add("node1", "node2", "node3", "node4", "node5")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Get(fmt.Sprintf("file%d.txt", i))
	}
}

func BenchmarkConsistentHash_GetN(b *testing.B) {
	ch := NewConsistentHash(150)
	ch.Add("node1", "node2", "node3", "node4", "node5")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.GetN(fmt.Sprintf("file%d.txt", i), 3)
	}
}
