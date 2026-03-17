# Go-DFS 后续改进计划

## 已完成 ✅

- [x] 代码优化（master.go、volume.go、client/main.go）
- [x] Master 单元测试（master_test.go）
- [x] Volume 单元测试（volume_test.go）
- [x] 单测教学文档（docs/testing-guide.md）
- [x] 一致性哈希实现（internal/hash/consistent_hash.go）
- [x] 一致性哈希测试（internal/hash/consistent_hash_test.go）
- [x] 集成到 Master（修改 AssignVolume 使用一致性哈希）
- [x] 一致性哈希原理文档（docs/consistent-hashing.md）
- [x] gRPC 连接池实现（internal/pool/grpc_pool.go）
- [x] 连接池测试（internal/pool/grpc_pool_test.go）
- [x] 集成到 Volume（修改 handleMetadata 使用连接池）
- [x] 连接池原理文档（docs/connection-pool.md）
- [x] **压测实现**（internal/bench/benchmark_test.go）
- [x] **压测教学文档**（docs/benchmark-guide.md）

---

## 待完成 📋

### 1. 性能优化 [优先级: 高]
- [ ] 优化 Master AssignVolume 性能
  - 当前: 1.9 ms/op (太慢)
  - 目标: < 100 μs/op
  - 方案: 禁用测试时持久化、使用 sync.Map
- [ ] 优化 Volume 读性能
  - 当前: 131 μs/op
  - 目标: < 50 μs/op
  - 方案: 增大 buffer、预读取

### 2. 并发优化 [优先级: 中]
- [ ] 锁粒度优化
  - 按文件分片锁
  - 或使用 sync.Map
- [ ] Volume 流式处理优化
  - 引入 pipeline 模式

### 3. 其他优化 [优先级: 低]
- [ ] 减少内存分配
  - 使用 sync.Pool
  - 复用 buffer
- [ ] 添加更多监控指标
  - QPS 统计
  - 延迟分布 (P50/P95/P99)

---

## 学习资源 📚

### 测试
- 本项目: docs/testing-guide.md
- Go 官方: https://go.dev/doc/tutorial/add-a-test

### 一致性哈希
- 本项目: docs/consistent-hashing.md
- 论文: Consistent Hashing and Random Trees

### 连接池
- 本项目: docs/connection-pool.md
- gRPC 最佳实践: https://grpc.io/docs/guides/performance/

### 压测
- 本项目: docs/benchmark-guide.md
- Go 官方: https://go.dev/doc/tutorial/add-a-test


---

## 性能优化完成 ✅

### 优化成果

| 指标 | 优化前 | 优化后 | 提升倍数 |
|------|--------|--------|----------|
| **延迟** | 2,000,244 ns/op | **2,127 ns/op** | **940x** |
| **内存分配** | 1,114,068 B/op | **388 B/op** | **2,870x** |
| **堆分配次数** | 10,829 allocs/op | **15 allocs/op** | **722x** |
| **QPS** | ~500 | ~470,000 | **940x** |

### 优化方案文档
- docs/performance-optimization.md

### 核心优化点
1. **sync.Map 替代 map+锁** - 无锁并发
2. **后台批量持久化** - 减少 IO 和 goroutine 创建
3. **内存路径跳过持久化** - 测试时快速返回

