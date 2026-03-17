package hash

import (
	"hash/crc32"
	"sort"
	"strconv"
	"sync"
)

// ConsistentHash 一致性哈希环
type ConsistentHash struct {
	mu        sync.RWMutex
	hashRing  []uint32          // 排序后的哈希值
	hashMap   map[uint32]string // 哈希值 -> 节点地址
	replicas  int               // 虚拟节点数
	nodeCount int               // 实际节点数
}

// NewConsistentHash 创建一致性哈希环
// replicas: 每个物理节点的虚拟节点数
func NewConsistentHash(replicas int) *ConsistentHash {
	if replicas <= 0 {
		replicas = 150 // 默认虚拟节点数
	}
	return &ConsistentHash{
		hashRing: make([]uint32, 0),
		hashMap:  make(map[uint32]string),
		replicas: replicas,
	}
}

// hash 计算字符串的哈希值
func (c *ConsistentHash) hash(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// Add 添加节点到哈希环
func (c *ConsistentHash) Add(nodes ...string) {
	if len(nodes) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, node := range nodes {
		if node == "" {
			continue
		}
		// 为每个物理节点创建 replicas 个虚拟节点
		for i := 0; i < c.replicas; i++ {
			// 虚拟节点 key: "node#0", "node#1", ...
			virtualKey := node + "#" + strconv.Itoa(i)
			hash := c.hash(virtualKey)
			c.hashRing = append(c.hashRing, hash)
			c.hashMap[hash] = node
		}
		c.nodeCount++
	}

	// 重新排序哈希环
	sort.Slice(c.hashRing, func(i, j int) bool {
		return c.hashRing[i] < c.hashRing[j]
	})
}

// Remove 从哈希环移除节点
func (c *ConsistentHash) Remove(node string) {
	if node == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 找到该节点的所有虚拟节点并移除
	newHashRing := make([]uint32, 0, len(c.hashRing)-c.replicas)
	for _, hash := range c.hashRing {
		if c.hashMap[hash] != node {
			newHashRing = append(newHashRing, hash)
		} else {
			delete(c.hashMap, hash)
		}
	}
	c.hashRing = newHashRing
	c.nodeCount--
}

// Get 获取 key 对应的节点
func (c *ConsistentHash) Get(key string) string {
	if key == "" {
		return ""
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.hashRing) == 0 {
		return ""
	}

	hash := c.hash(key)

	// 二分查找第一个 >= hash 的位置
	idx := sort.Search(len(c.hashRing), func(i int) bool {
		return c.hashRing[i] >= hash
	})

	// 如果超出范围，回到环的起点
	if idx == len(c.hashRing) {
		idx = 0
	}

	return c.hashMap[c.hashRing[idx]]
}

// GetN 获取 key 对应的 N 个不同节点（用于副本）
func (c *ConsistentHash) GetN(key string, n int) []string {
	if key == "" || n <= 0 {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.hashRing) == 0 {
		return nil
	}

	// 如果请求的节点数大于实际节点数，返回所有节点
	if n > c.nodeCount {
		n = c.nodeCount
	}

	hash := c.hash(key)
	idx := sort.Search(len(c.hashRing), func(i int) bool {
		return c.hashRing[i] >= hash
	})

	// 收集 N 个不同的节点
	result := make([]string, 0, n)
	seen := make(map[string]bool)

	for len(result) < n {
		if idx >= len(c.hashRing) {
			idx = 0
		}
		node := c.hashMap[c.hashRing[idx]]
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
		idx++
	}

	return result
}

// GetAllNodes 返回所有物理节点
func (c *ConsistentHash) GetAllNodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := make(map[string]bool)
	result := make([]string, 0, c.nodeCount)

	for _, node := range c.hashMap {
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
	}

	return result
}

// NodeCount 返回物理节点数
func (c *ConsistentHash) NodeCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeCount
}

// VirtualCount 返回虚拟节点数
func (c *ConsistentHash) VirtualCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.hashRing)
}
