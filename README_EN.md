# Building a Distributed File System from Scratch

A simplified distributed file system implementation in Go, built for learning core distributed systems concepts.

---

## Introduction

This is a simplified distributed file system (DFS) implementation inspired by industrial systems like GFS, HDFS, and Ceph, but heavily simplified for educational purposes.

The main goal is to understand core distributed systems concepts:
- Metadata management
- Data sharding and replication
- Consistent hashing
- Connection pooling
- Performance optimization

---

## Architecture

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

## Core Features

### Implemented

- Master Node Management
  - Volume node registration and heartbeat
  - File allocation scheduling (consistent hashing)
  - Metadata management

- Volume Node Storage
  - File upload (with chain replication)
  - File download
  - Data forwarding

- Distributed Features
  - 3-replica redundancy
  - Consistent hashing (virtual nodes)
  - gRPC connection pool

- Performance Optimization
  - sync.Map lock-free concurrency
  - Background batch persistence
  - 1000x performance improvement

### TODO

- Master high availability (Raft)
- Automatic data migration
- Data integrity check (CRC)
- Monitoring metrics

---

## Quick Start

### Requirements

- Go 1.21+
- Protocol Buffers (protoc)

### Build

```bash
# Generate protobuf code
make gen

# Build all components
make build

# Or build everything
make all
```

### Run

1. Start Master
```bash
./bin/master -port=50051
```

2. Start Volume (3 nodes)
```bash
./bin/volume -id=vol-1 -port=50052 -master=localhost:50051
./bin/volume -id=vol-2 -port=50053 -master=localhost:50051
./bin/volume -id=vol-3 -port=50054 -master=localhost:50051
```

3. Use Client to upload/download
```bash
go run cmd/client/main.go -action=upload -file=test.txt -master=localhost:50051
go run cmd/client/main.go -action=download -file=test.txt -master=localhost:50051
```

---

## Project Structure

```
go-dfs/
├── api/                    # Protocol Buffers definitions
│   └── dfs.proto
├── cmd/                    # Executable entry points
│   ├── master/            # Master node
│   ├── volume/            # Volume node
│   └── client/            # Client tool
├── internal/              # Internal implementations
│   ├── hash/             # Consistent hashing
│   ├── pool/             # gRPC connection pool
│   ├── service/          # Master/Volume service logic
│   └── bench/            # Benchmarks
├── docs/                  # Technical documentation
│   ├── testing-guide.md
│   ├── consistent-hashing.md
│   ├── connection-pool.md
│   ├── benchmark-guide.md
│   └── performance-optimization.md
├── Makefile
└── README.md
```

---

## Technical Documentation

- [Testing Guide](docs/testing-guide.md)
- [Consistent Hashing](docs/consistent-hashing.md)
- [Connection Pool Design](docs/connection-pool.md)
- [Benchmark Guide](docs/benchmark-guide.md)
- [Performance Optimization](docs/performance-optimization.md)

---

## Performance

### Before vs After Optimization

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Master scheduling latency | 2 ms | 2 us | 1000x |
| Memory allocation | 1.1 MB | 388 B | 2870x |
| Consistent hash lookup | - | 50 ns | - |

---

## Screenshot

(Place screenshot here)

---

## Learning Path

If you are new to distributed systems, read in this order:

1. [Testing Guide](docs/testing-guide.md) to understand project structure
2. [Consistent Hashing](docs/consistent-hashing.md) for core algorithms
3. [Performance Optimization](docs/performance-optimization.md) for optimization ideas
4. Modify code and add new features

---

## References

- [The Google File System](https://static.googleusercontent.com/media/research.google.com/en//archive/gfs-sosp2003.pdf)
- [HDFS Architecture Guide](https://hadoop.apache.org/docs/stable/hadoop-project-dist/hadoop-hdfs/HdfsDesign.html)
- [Ceph Architecture](https://docs.ceph.com/en/latest/architecture/)

---

## License

MIT License

---

## Author

Sophomore student, distributed systems learner.

---

## Chinese Version

See [README.md](README.md)
