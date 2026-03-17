# Go-DFS 后续改进计划

## 已完成 ✅

- [x] 代码优化（master.go、volume.go、client/main.go）
- [x] Master 单元测试（master_test.go）
- [x] Volume 单元测试（volume_test.go）
- [x] 单测教学文档（docs/testing-guide.md）
- [x] **一致性哈希实现**（internal/hash/consistent_hash.go）
- [x] **一致性哈希测试**（internal/hash/consistent_hash_test.go）
- [x] **集成到 Master**（修改 AssignVolume 使用一致性哈希）
- [x] **原理讲解文档**（docs/consistent-hashing.md）

---

## 待完成 📋

### 1. 集成测试 [优先级: 高]
- [ ] 创建 integration_test.go
  - 启动真实 Master gRPC 服务
  - 启动真实 Volume gRPC 服务
  - 测试完整上传流程
  - 测试完整下载流程
  - 测试多节点场景
- [ ] 使用 bufconn 进行内存通信测试
  - 避免占用真实端口
  - 加速测试执行

### 2. 连接池/线程池 [优先级: 中]
- [ ] gRPC 连接池实现
  - 复用 Volume 间转发连接
  - 最大连接数限制
  - 连接健康检查
  - 参考: github.com/processout/grpc-go-pool
- [ ] 工作线程池
  - 限制并发上传数量
  - 任务队列管理
- [ ] 配置化参数
  - 池大小可配置
  - 超时时间可配置

### 3. 并发优化 [优先级: 中]
- [ ] 锁粒度优化
  - 按文件分片锁
  - 或使用 sync.Map
  - 读写分离优化
- [ ] Volume 流式处理优化
  - 引入 pipeline 模式
  - 并行读写

### 4. 压力测试 [优先级: 低]
- [ ] 基准测试
  - 使用 go test -bench
  - 测试 Master 调度性能
  - 测试 Volume 读写性能
- [ ] 压力测试工具
  - 使用 ghz 进行 gRPC 压测
  - 或使用 vegeta 进行 HTTP 压测（如有）
- [ ] 监控指标
  - QPS
  - P50/P95/P99 延迟
  - 内存占用
  - GC 频率
- [ ] 性能分析报告

---

## 学习资源 📚

### 一致性哈希
- 本项目文档: docs/consistent-hashing.md
- 论文: Consistent Hashing and Random Trees
- 文章: https://medium.com/system-design-blog/consistent-hashing-b7dd1d96d775
- 代码参考: groupcache/consistenthash.go

### gRPC 连接池
- 官方文档: https://grpc.io/docs/guides/performance/
- 最佳实践: https://github.com/grpc/grpc-go/blob/master/Documentation/keepalive.md

### Go 并发模式
- 文章: https://go.dev/blog/pipelines
- 书籍: Go Concurrency Patterns

### 测试
- 本项目文档: docs/testing-guide.md
- Go 官方: https://go.dev/doc/tutorial/add-a-test
- 高级测试: https://go.dev/blog/subtests

---

## 执行建议

1. 先完成集成测试，确保端到端功能稳定
2. 再实现连接池，优化转发性能
3. 并发优化可以并行进行
4. 最后做压测，验证优化效果

