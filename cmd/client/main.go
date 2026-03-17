package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"go-dfs/api"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// 默认缓冲区大小 64KB
	defaultBufferSize = 64 * 1024
	// 默认超时时间
	defaultTimeout = 30 * time.Second
)

func main() {
	var (
		masterAddr = flag.String("master", "localhost:50051", "Master 地址")
		action     = flag.String("action", "upload", "操作: upload/download")
		filename   = flag.String("file", "", "文件名")
		localPath  = flag.String("path", "", "本地文件路径")
	)
	flag.Parse()

	if *filename == "" {
		log.Fatal("请指定文件名: -file=<name>")
	}

	// 连接 Master
	masterConn, err := grpc.NewClient(*masterAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接 Master: %v", err)
	}
	defer masterConn.Close()

	mClient := api.NewMasterServiceClient(masterConn)

	switch *action {
	case "upload":
		if *localPath == "" {
			*localPath = *filename
		}
		if err := uploadFile(mClient, *filename, *localPath); err != nil {
			log.Fatalf("上传失败: %v", err)
		}
		log.Println("上传成功")

	case "download":
		if *localPath == "" {
			*localPath = "downloaded_" + *filename
		}
		if err := downloadFile(mClient, *filename, *localPath); err != nil {
			log.Fatalf("下载失败: %v", err)
		}
		log.Println("下载成功")

	default:
		log.Fatalf("未知操作: %s", *action)
	}
}

// uploadFile 上传文件
func uploadFile(mClient api.MasterServiceClient, filename, localPath string) error {
	// 获取文件信息
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("无法访问文件: %v", err)
	}

	// 向 Master 申请存储节点
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	resp, err := mClient.AssignVolume(ctx, &api.AssignVolumeRequest{
		Filename: filename,
		FileSize: fileInfo.Size(),
	})
	if err != nil {
		return fmt.Errorf("申请存储节点失败: %v", err)
	}

	log.Printf("Master 分配节点: %v", resp.Address)

	// 连接第一个 Volume 节点
	vConn, err := grpc.NewClient(resp.Address[0], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("无法连接 Volume: %v", err)
	}
	defer vConn.Close()

	vClient := api.NewVolumeServiceClient(vConn)
	return uploadInStream(vClient, filename, localPath, resp.Address)
}

// uploadInStream 流式上传文件
func uploadInStream(client api.VolumeServiceClient, filename string, filePath string, targets []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	stream, err := client.UploadFile(ctx)
	if err != nil {
		return fmt.Errorf("无法建立上传流: %v", err)
	}

	// 发送元数据
	if err := stream.Send(&api.UploadRequest{
		Data: &api.UploadRequest_Metadata{
			Metadata: &api.Metadata{
				Filename:    filename,
				NextTargets: targets,
			},
		},
	}); err != nil {
		return fmt.Errorf("发送元数据失败: %v", err)
	}

	// 打开本地文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 流式发送
	buffer := make([]byte, defaultBufferSize)
	sent := int64(0)

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取文件出错: %v", err)
		}

		if err := stream.Send(&api.UploadRequest{
			Data: &api.UploadRequest_Chunk{Chunk: buffer[:n]},
		}); err != nil {
			return fmt.Errorf("流式发送中断: %v", err)
		}
		sent += int64(n)
	}

	// 关闭流并接收响应
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("接收服务端回执失败: %v", err)
	}

	if !resp.Success {
		return fmt.Errorf("服务端返回失败")
	}

	log.Printf("上传成功！文件 ID: %s, 大小: %d bytes", resp.FileId, sent)
	return nil
}

// downloadFile 下载文件
func downloadFile(mClient api.MasterServiceClient, filename, localPath string) error {
	// 获取文件位置
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	locResp, err := mClient.GetFileLocation(ctx, &api.FileLocationRequest{
		Filename: filename,
	})
	if err != nil {
		return fmt.Errorf("获取文件位置失败: %v", err)
	}

	log.Printf("从节点 %s 下载文件", locResp.Address)

	// 连接 Volume 节点
	vConn, err := grpc.NewClient(locResp.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("无法连接 Volume: %v", err)
	}
	defer vConn.Close()

	vClient := api.NewVolumeServiceClient(vConn)
	return downloadInStream(vClient, filename, localPath)
}

// downloadInStream 流式下载文件
func downloadInStream(client api.VolumeServiceClient, filename, localPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	stream, err := client.DownloadFile(ctx, &api.DownloadRequest{Filename: filename})
	if err != nil {
		return fmt.Errorf("发起下载失败: %v", err)
	}

	// 创建本地文件
	outFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer outFile.Close()

	// 接收数据
	received := int64(0)
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("接收中断: %v", err)
		}

		if _, err := outFile.Write(resp.Content); err != nil {
			return fmt.Errorf("写入文件失败: %v", err)
		}
		received += int64(len(resp.Content))
	}

	log.Printf("下载完成，共 %d bytes", received)
	return nil
}
