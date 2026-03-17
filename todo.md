# Go-DFS 后续改进计划

## 已完成 ✅

- [x] 代码优化（master.go、volume.go、client/main.go）
- [x] 单元测试（master_test.go）
- [x] 基础功能验证

---

## 待完成 📋

### 1. 单元测试完善 [优先级: 高]
- [ ] 添加 volume_test.go
  - 测试 UploadFile 正常流程
  - 测试 DownloadFile 正常流程
  - 测试元数据缺失场景
  - 测试文件不存在场景
  - 测试链式转发
- [ ] 添加集成测试
  - 启动真实 gRPC 服务
  - 端到端上传下载测试
  - 多节点场景测试
- [ ] 添加 mock 测试
  - 使用 mock 测试 gRPC 客户端
  - 模拟网络超时场景

### 2. 一致性哈希 [优先级: 高]
- [ ] 调研 consistent hashing 算法
  - 推荐库: github.com/buraksezer/consistent
  - 或参考 groupcache 实现
- [ ] 实现 ConsistentHashRing 结构
  - 虚拟节点（vnode）支持
  - 添加/删除节点
  - 获取文件对应节点
- [ ] 替换现有轮询算法
  - 修改 AssignVolume 逻辑
  - 保持向后兼容
- [ ] 测试节点增删时的数据迁移

### 3. 并发优化 [优先级: 中]
- [ ] 锁粒度优化
  - 按文件分片锁
  - 或使用 sync.Map
  - 读写分离优化
- [ ] Volume 流式处理优化
  - 引入 pipeline 模式
  - 并行读写
- [ ] 无锁数据结构调研
  - ring buffer
  - lock-free queue

### 4. 连接池/线程池 [优先级: 中]
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

### 5. 压力测试 [优先级: 低]
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
- 论文: Consistent Hashing and Random Trees
- 文章: https://medium.com/system-design-blog/consistent-hashing-b7dd1d96d775
- 代码参考: groupcache/consistenthash.go

### gRPC 连接池
- 官方文档: https://grpc.io/docs/guides/performance/
- 最佳实践: https://github.com/grpc/grpc-go/blob/master/Documentation/keepalive.md

### Go 并发模式
- 文章: https://go.dev/blog/pipelines
- 书籍: Go Concurrency Patterns

---

## 执行建议

1. 先完成单元测试，确保基础功能稳定
2. 再实现一致性哈希，解决节点扩展问题
3. 并发优化和连接池可以并行进行
4. 最后做压测，验证优化效果

