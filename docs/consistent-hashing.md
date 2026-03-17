# 一致性哈希完全指南

## 一、问题背景

### 传统哈希的问题
```
节点数 = 3
file1.txt -> hash(file1) % 3 = 1 -> Node1
file2.txt -> hash(file2) % 3 = 2 -> Node2
file3.txt -> hash(file3) % 3 = 0 -> Node0
```

**问题：当节点数变为 4 时**
```
file1.txt -> hash(file1) % 4 = 1 -> Node1 (没变 ✓)
file2.txt -> hash(file2) % 4 = 2 -> Node2 (没变 ✓)
file3.txt -> hash(file3) % 4 = 0 -> Node0 (没变 ✓)
file4.txt -> hash(file4) % 4 = 3 -> Node3 (新节点)
```

看起来没问题？再试一个：
```
file5.txt -> hash(file5) % 3 = 1 -> Node1
file5.txt -> hash(file5) % 4 = 1 -> Node1 (没变)

file6.txt -> hash(file6) % 3 = 2 -> Node2
file6.txt -> hash(file6) % 4 = 3 -> Node3 (变了！需要迁移)
```

**结论**：传统哈希在节点变化时，**几乎所有数据都要重新映射**，迁移成本极高。

---

## 二、一致性哈希原理

### 2.1 核心思想
> 把哈希空间想象成一个环（0 ~ 2³²-1），节点和数据都映射到这个环上。

```
          0 (2³²)
          │
    Node2 ●
         / \
        /   \
    Data1    \
      /       \
     /         \
    /           \
   /      Node0  \
  ●───────────────●
  │               │
Data2           Node1
  │               │
  └───────────────┘
```

### 2.2 数据定位规则
**数据存储在顺时针方向的第一个节点上**

```
          0
          │
    Node2 ●
         /|\
        / | \
    Data1  |  \
      /    |   \
     /     |    \
    /      |     \
   /       |      \
  ●────────┼───────●
  │        |       │
Data2      |     Node1
  │        |       │
  └────────┴───────┘

Data1 顺时针第一个节点是 Node0
Data2 顺时针第一个节点是 Node1
```

### 2.3 节点增删时的影响

**添加 Node3：**
```
          0
          │
    Node2 ●
         / \
        /   \
    Data1    \
      /       \
     /    Node3●  ← 新节点
    /         / \
   /         /   \
  ●────────/─────●
  │        /     │
Data2     /    Node1
  │      /       │
  └─────┴────────┘

只有原本属于 Node0 的部分数据需要迁移到 Node3
其他数据（Data1、Data2）不受影响！
```

**数据迁移率 = 1/N**（N 是节点数）

---

## 三、虚拟节点

### 3.1 为什么要虚拟节点？

**问题：节点分布不均匀**
```
          0
          │
    Node2 ●
         / \
        /   \
       /     \
      /       \
     /         \
    /           \
   /             \
  ●───────────────●
  │               │
  │             Node1
  │               │
  └───────────────┘

Node0 和 Node2 很近，Node1 独占大半空间
→ 负载不均衡
```

### 3.2 虚拟节点解决方案
> 每个物理节点对应多个虚拟节点，分散在环上。

```
Node1 → vnode1#0, vnode1#1, vnode1#2, ...
Node2 → vnode2#0, vnode2#1, vnode2#2, ...
Node3 → vnode3#0, vnode3#1, vnode3#2, ...

          0
          │
   v1#0  ●
         / \
   v2#1 ●   \
        |    \
   v3#2●      \
        |       \
   v1#1●         \
        |          \
   v2#0●            \
        |             \
   v3#0●───────────────●
        |               │
   v3#1●             v1#2
        |               │
   v2#2●             v3#3
        |               │
        └───────────────┘

每个物理节点的虚拟节点均匀分布
→ 负载均衡
```

### 3.3 虚拟节点数量
- 通常 100~200 个虚拟节点/物理节点
- 越多越均匀，但内存和查找成本增加
- 本项目使用 150 个

---

## 四、代码实现详解

### 4.1 数据结构
```go
type ConsistentHash struct {
    mu        sync.RWMutex
    hashRing  []uint32          // 排序后的哈希值（环）
    hashMap   map[uint32]string // 哈希值 -> 物理节点
    replicas  int               // 虚拟节点数
    nodeCount int               // 物理节点数
}
```

### 4.2 添加节点
```go
func (c *ConsistentHash) Add(nodes ...string) {
    for _, node := range nodes {
        // 为每个物理节点创建 replicas 个虚拟节点
        for i := 0; i < c.replicas; i++ {
            // 虚拟节点 key: "node#0", "node#1", ...
            virtualKey := node + "#" + strconv.Itoa(i)
            hash := c.hash(virtualKey)  // 计算哈希
            
            // 加入环
            c.hashRing = append(c.hashRing, hash)
            c.hashMap[hash] = node  // 记录映射
        }
    }
    
    // 排序，形成环
    sort.Slice(c.hashRing, ...)
}
```

**示例：**
```
添加 Node1:
  hash("Node1#0") = 100 → hashRing=[100], hashMap[100]=Node1
  hash("Node1#1") = 500 → hashRing=[100,500], hashMap[500]=Node1
  hash("Node1#2") = 900 → hashRing=[100,500,900], hashMap[900]=Node1

添加 Node2:
  hash("Node2#0") = 300 → hashRing=[100,300,500,900]
  hash("Node2#1") = 700 → hashRing=[100,300,500,700,900]
  ...
```

### 4.3 查找节点（核心算法）
```go
func (c *ConsistentHash) Get(key string) string {
    hash := c.hash(key)  // 计算 key 的哈希
    
    // 二分查找：第一个 >= hash 的位置
    idx := sort.Search(len(c.hashRing), func(i int) bool {
        return c.hashRing[i] >= hash
    })
    
    // 如果超出范围，回到环的起点（环形）
    if idx == len(c.hashRing) {
        idx = 0
    }
    
    // 返回对应的物理节点
    return c.hashMap[c.hashRing[idx]]
}
```

**查找过程图示：**
```
hashRing = [100, 300, 500, 700, 900]
hashMap = {
    100: Node1, 300: Node2, 500: Node1,
    700: Node2, 900: Node1
}

查找 "file.txt" (hash=400):
    第一个 >= 400 的是 500
    hashMap[500] = Node1
    → 返回 Node1

查找 "doc.txt" (hash=950):
    没有 >= 950 的
    idx = 5 == len(hashRing)
    回到起点，返回 hashMap[100] = Node1

查找 "data.txt" (hash=100):
    第一个 >= 100 的是 100
    返回 hashMap[100] = Node1
```

### 4.4 获取 N 个节点（副本）
```go
func (c *ConsistentHash) GetN(key string, n int) []string {
    hash := c.hash(key)
    idx := sort.Search(...)  // 找到起始位置
    
    result := make([]string, 0, n)
    seen := make(map[string]bool)
    
    // 顺时针收集 N 个不同的物理节点
    for len(result) < n {
        if idx >= len(c.hashRing) {
            idx = 0  // 环形
        }
        
        node := c.hashMap[c.hashRing[idx]]
        if !seen[node] {  // 去重
            seen[node] = true
            result = append(result, node)
        }
        idx++
    }
    
    return result
}
```

**示例：**
```
hashRing = [100, 300, 500, 700, 900]
hashMap = {100:Node1, 300:Node2, 500:Node1, 700:Node2, 900:Node1}

GetN("file.txt", 3):
    hash(file.txt) = 400
    起始位置：500 (idx=2)
    
    idx=2: hashMap[500]=Node1, 加入结果 [Node1]
    idx=3: hashMap[700]=Node2, 加入结果 [Node1, Node2]
    idx=4: hashMap[900]=Node1, 已存在，跳过
    idx=5: >= len, 回到 0
    idx=0: hashMap[100]=Node1, 已存在，跳过
    idx=1: hashMap[300]=Node2, 已存在，跳过
    idx=2: hashMap[500]=Node1, 已存在，跳过
    ...
    
    只有 2 个不同节点，返回 [Node1, Node2]
```

### 4.5 移除节点
```go
func (c *ConsistentHash) Remove(node string) {
    // 遍历环，移除该节点的所有虚拟节点
    newHashRing := make([]uint32, 0)
    for _, hash := range c.hashRing {
        if c.hashMap[hash] != node {
            newHashRing = append(newHashRing, hash)
        } else {
            delete(c.hashMap, hash)
        }
    }
    c.hashRing = newHashRing
}
```

---

## 五、性能分析

### 5.1 时间复杂度
| 操作 | 复杂度 | 说明 |
|------|--------|------|
| Add | O(replicas × log N) | 添加虚拟节点 + 排序 |
| Remove | O(N) | 遍历所有虚拟节点 |
| Get | O(log N) | 二分查找 |
| GetN | O(N) | 最坏情况遍历整个环 |

N = 虚拟节点总数 = 物理节点数 × replicas

### 5.2 空间复杂度
O(N) - 存储所有虚拟节点的哈希值

### 5.3 实际性能（本项目）
```
BenchmarkConsistentHash_Get-16    9059947    136.4 ns/op    40 B/op    3 allocs/op
BenchmarkConsistentHash_GetN-16   5014056    241.9 ns/op    88 B/op    4 allocs/op
```

单次查找约 136 纳秒，非常快！

---

## 六、与传统轮询对比

| 特性 | 传统轮询 | 一致性哈希 |
|------|----------|------------|
| 相同 key 映射 | 变化（轮询） | 固定 |
| 节点增删影响 | 所有数据重新分配 | 仅 1/N 数据迁移 |
| 负载均衡 | 完美 | 近似（依赖虚拟节点） |
| 实现复杂度 | 简单 | 中等 |
| 适用场景 | 节点固定 | 节点动态变化 |

---

## 七、应用场景

1. **分布式缓存**（Memcached、Redis Cluster）
2. **分布式存储**（Ceph、Swift）
3. **负载均衡**（Nginx、HAProxy）
4. **数据库分片**（MongoDB、Cassandra）
5. **P2P 网络**（DHT、BitTorrent）

---

## 八、常见问题

### Q1: 虚拟节点越多越好吗？
不是。越多越均匀，但：
- 内存占用增加
- 查找时间增加
- Add/Remove 变慢

通常 100-200 是 sweet spot。

### Q2: 哈希冲突怎么办？
CRC32 冲突概率极低。如果真的冲突：
- 使用更好的哈希函数（Murmur3、SHA256）
- 冲突时加随机盐重新哈希

### Q3: 节点权重不同怎么办？
给权重高的节点更多虚拟节点：
```
Node1 (权重 2) → 200 个虚拟节点
Node2 (权重 1) → 100 个虚拟节点
```

### Q4: 如何实现数据迁移？
```
// 添加节点时
newNode := "Node4"
ch.Add(newNode)

// 遍历所有数据，检查是否需要迁移
for file, oldNode := range fileMap {
    newAssigned := ch.Get(file)
    if newAssigned == newNode && oldNode != newNode {
        // 需要迁移：从 oldNode 迁移到 newNode
        migrate(file, oldNode, newNode)
    }
}
```

---

## 九、可视化理解

```
# 3 个节点，每个 4 个虚拟节点

        0 (2³²)
        │
   N1#0 ●─────┐
        │     │
   N2#2 ●     │
        │     │
   N3#1 ●     │
        │     │
   N1#2 ●     │
        │     │
   N2#0 ●     │
        │     │
   N3#3 ●     │
        │     │
   N1#1 ●     │
        │     │
   N2#3 ●     │
        │     │
   N3#0 ●─────┘
        │
   N2#1 ●
        │
   N3#2 ●
        │
        └─────┘

数据 hash 落在哪个区间，就归该虚拟节点对应的物理节点管理
```

---

## 十、本项目使用

```go
// 创建哈希环
hashRing := hash.NewConsistentHash(150)

// 注册节点时添加
hashRing.Add("127.0.0.1:50052")
hashRing.Add("127.0.0.1:50053")
hashRing.Add("127.0.0.1:50054")

// 分配文件存储位置
nodes := hashRing.GetN("file.txt", 3)
// 返回 ["127.0.0.1:50052", "127.0.0.1:50054", "127.0.0.1:50053"]

// 节点下线时移除
hashRing.Remove("127.0.0.1:50052")
// 只有原本映射到 50052 的文件需要迁移
```

---

## 十一、学习检查清单

- [ ] 理解传统哈希在节点变化时的问题
- [ ] 理解一致性哈希的环形空间概念
- [ ] 理解"顺时针第一个节点"的定位规则
- [ ] 理解虚拟节点的作用
- [ ] 能画出节点增删时的数据迁移示意图
- [ ] 理解代码中二分查找的逻辑
- [ ] 知道时间复杂度和空间复杂度
- [ ] 了解常见应用场景

---

## 十二、动手练习

1. **修改虚拟节点数**：把 replicas 从 150 改成 10，观察负载分布变化
2. **实现带权重的哈希环**：让某些节点有更多虚拟节点
3. **实现数据迁移统计**：添加节点时计算需要迁移的数据比例
4. **可视化工具**：写一个程序打印哈希环的 ASCII 图

