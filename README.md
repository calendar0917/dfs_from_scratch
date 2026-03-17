# 从零开始的分布式文件系统

一个用 Go 语言实现的简化版分布式文件系统，用于学习分布式系统的核心概念。

---

## English Version

See [README_EN.md](README_EN.md)

## 项目简介

这是一个参考 GFS、HDFS、Ceph 等工业级系统设计思想，但做了大量简化以便学习的分布式文件系统实现。

项目的主要目标是帮助理解分布式系统的核心概念：
- 元数据管理
- 数据分片与副本
- 一致性哈希
- 连接池
- 性能优化

[png](./pic1.png)

---

## 架构设计

```
+-------------------+        +-------------------+        +-------------------+
|     Client        |        |      Master       |        |      Volume       |
|                   |        |                   |        |                   |
|  - Upload files   | <----> |  - Node registry  | <----> |  - Store files    |
|  - Download files |        |  - File allocation|        |  - Forward data   |
|                   |        |  - Metadata mgmt  |        |                   |
+-------------------+        +-------------------+        +-------------------+
                                    |    |    |
                                    v    v    v
                              +-------------------+
                              |   Volume Nodes    |
                              |  (Data replicas)  |
                              +-------------------+
```

---

## 核心功能

### 已实现

- Master 节点管理
  - Volume 节点注册与心跳检测
  - 文件分配调度（一致性哈希）
  - 元数据管理

- Volume 节点存储
  - 文件上传（支持链式复制）
  - 文件下载
  - 数据转发

- 分布式特性
  - 3 副本冗余
  - 一致性哈希（虚拟节点）
  - gRPC 连接池

- 性能优化
  - sync.Map 无锁并发
  - 后台批量持久化
  - 性能提升 1000x

### 待实现

- Master 高可用（Raft）
- 数据自动迁移
- 数据校验（CRC）
- 监控指标

---

## 快速开始

### 环境要求

- Go 1.21+
- Protocol Buffers (protoc)

### 编译

```bash
# 生成 protobuf 代码
make gen

# 编译所有组件
make build

# 或者直接编译
make all
```

### 运行

1. 启动 Master
```bash
./bin/master -port=50051
```

2. 启动 Volume（启动 3 个节点）
```bash
./bin/volume -id=vol-1 -port=50052 -master=localhost:50051
./bin/volume -id=vol-2 -port=50053 -master=localhost:50051
./bin/volume -id=vol-3 -port=50054 -master=localhost:50051
```

3. 使用 Client 上传/下载文件
```bash
go run cmd/client/main.go -action=upload -file=test.txt -master=localhost:50051
go run cmd/client/main.go -action=download -file=test.txt -master=localhost:50051
```

---

## 项目结构

```
go-dfs/
├── api/                    # Protocol Buffers 定义
│   └── dfs.proto
├── cmd/                    # 可执行程序入口
│   ├── master/            # Master 节点
│   ├── volume/            # Volume 节点
│   └── client/            # 客户端工具
├── internal/              # 内部实现
│   ├── hash/             # 一致性哈希实现
│   ├── pool/             # gRPC 连接池
│   ├── service/          # Master/Volume 服务逻辑
│   └── bench/            # 性能测试
├── docs/                  # 技术文档
│   ├── testing-guide.md
│   ├── consistent-hashing.md
│   ├── connection-pool.md
│   ├── benchmark-guide.md
│   └── performance-optimization.md
├── Makefile
└── README.md
```

---

## 技术文档

- [单元测试指南](docs/testing-guide.md)
- [一致性哈希原理](docs/consistent-hashing.md)
- [连接池设计与实现](docs/connection-pool.md)
- [性能测试指南](docs/benchmark-guide.md)
- [性能优化实战](docs/performance-optimization.md)

---

## 性能数据

### 优化前后对比

| 指标 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| Master 调度延迟 | 2 ms | 2 us | 1000x |
| 内存分配 | 1.1 MB | 388 B | 2870x |
| 一致性哈希查找 | - | 50 ns | - |

---

## 运行截图

（此处放置运行截图）

---

## 学习建议

如果你是分布式系统的初学者，建议按以下顺序阅读：

1. 先阅读 [单元测试指南](docs/testing-guide.md) 了解项目结构
2. 阅读 [一致性哈希原理](docs/consistent-hashing.md) 理解核心算法
3. 阅读 [性能优化实战](docs/performance-optimization.md) 学习优化思路
4. 动手修改代码，添加新功能

---

## 参考资源

- [The Google File System](https://static.googleusercontent.com/media/research.google.com/en//archive/gfs-sosp2003.pdf)
- [HDFS Architecture Guide](https://hadoop.apache.org/docs/stable/hadoop-project-dist/hadoop-hdfs/HdfsDesign.html)
- [Ceph Architecture](https://docs.ceph.com/en/latest/architecture/)

---

## 许可证

MIT License

---

