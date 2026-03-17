# Go 单元测试完全指南

## 一、测试基础

### 1.1 测试文件命名
```
xxx.go      -> xxx_test.go
master.go   -> master_test.go
volume.go   -> volume_test.go
```

### 1.2 基本测试结构
```go
package service  // 与被测代码同包

import "testing"

// 测试函数必须以 Test 开头
func TestXxx(t *testing.T) {
    // 测试代码
}
```

### 1.3 运行测试
```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/service/...

# 运行特定测试函数
go test -run TestRegisterNode

# 运行特定子测试
go test -run "TestRegisterNode/新节点注册"

# 显示详细输出
go test -v

# 生成覆盖率报告
go test -cover
```

---

## 二、核心测试模式

### 2.1 表驱动测试（最常用）
```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"正数相加", 1, 2, 3},
        {"负数相加", -1, -2, -3},
        {"零相加", 0, 0, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Add(tt.a, tt.b)
            if result != tt.expected {
                t.Errorf("Add(%d, %d) = %d; want %d", 
                    tt.a, tt.b, result, tt.expected)
            }
        })
    }
}
```

### 2.2 子测试（t.Run）
```go
func TestMasterServer(t *testing.T) {
    t.Run("注册节点", func(t *testing.T) {
        // 测试注册逻辑
    })
    
    t.Run("心跳检测", func(t *testing.T) {
        // 测试心跳逻辑
    })
}
```

### 2.3 测试辅助函数
```go
func setupTestServer(t *testing.T) (*MasterServer, func()) {
    tmpDir := t.TempDir()  // 自动清理的临时目录
    s := NewMasterServer(filepath.Join(tmpDir, "test.db"))
    
    cleanup := func() {
        os.RemoveAll(tmpDir)
    }
    
    return s, cleanup
}

// 使用
func TestXxx(t *testing.T) {
    s, cleanup := setupTestServer(t)
    defer cleanup()  // 确保清理
    // 测试...
}
```

---

## 三、Mock 技术

### 3.1 为什么要 Mock？
被测代码依赖外部服务时，Mock 可以：
- 控制返回值
- 模拟错误场景
- 避免真实网络调用

### 3.2 gRPC Mock 示例
```go
// mockUploadStream 模拟上传流
type mockUploadStream struct {
    grpc.ServerStream
    requests  []*api.UploadRequest
    response  *api.UploadResponse
    recvIndex int
}

func (m *mockUploadStream) Recv() (*api.UploadRequest, error) {
    if m.recvIndex >= len(m.requests) {
        return nil, io.EOF  // 模拟流结束
    }
    req := m.requests[m.recvIndex]
    m.recvIndex++
    return req, nil
}

func (m *mockUploadStream) SendAndClose(resp *api.UploadResponse) error {
    m.response = resp
    return nil
}
```

---

## 四、测试最佳实践

### 4.1 AAA 模式
```go
func TestXxx(t *testing.T) {
    // Arrange: 准备
    s := NewServer()
    input := "test"
    
    // Act: 执行
    result, err := s.Process(input)
    
    // Assert: 验证
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
}
```

### 4.2 命名规范
```go
// 测试函数：Test + 被测函数 + 场景
TestRegisterNode_Success
TestRegisterNode_EmptyID

// 子测试：描述行为
"新节点注册成功"
"空 node_id 返回错误"
```

---

## 五、常见问题

### Q: 测试覆盖率要达到多少？
- 核心逻辑：>80%
- 工具函数：>60%

### Q: 如何测试私有函数？
- 方案 1：测试同包（`_test.go` 与代码同包）
- 方案 2：只测试公有 API（推荐）

### Q: 测试太慢怎么办？
- 使用 `t.Parallel()` 并行执行
- Mock 外部依赖

