package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"go-dfs/api" // 替换成你的模块名

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func uploadInStream(client api.VolumeServiceClient, filename string, filePath string, targets []string) error {
	// 1. 建立流（增加超时控制）
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	stream, err := client.UploadFile(ctx)
	if err != nil {
		return fmt.Errorf("无法建立上传流: %v", err)
	}

	// 2. 发送元数据
	err = stream.Send(&api.UploadRequest{
		Data: &api.UploadRequest_Metadata{
			Metadata: &api.Metadata{Filename: filename, NextTargets: targets},
		},
	})
	if err != nil {
		return fmt.Errorf("发送元数据失败: %v", err)
	}

	// 3. 打开文件（检查路径！）
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 4. 循环发送
	buffer := make([]byte, 64*1024)
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取文件出错: %v", err)
		}

		// 检查发送错误，这是防止 CPU 飙升的关键！
		if err := stream.Send(&api.UploadRequest{
			Data: &api.UploadRequest_Chunk{
				Chunk: buffer[:n],
			},
		}); err != nil {
			return fmt.Errorf("流式发送中断: %v", err)
		}
	}

	// 5. 关闭并接收回复
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("接收服务端回执失败: %v", err)
	}

	log.Printf("上传成功！文件 ID: %s", resp.FileId)
	return nil
}

func getFileLocation(client api.MasterServiceClient, filename string) string {
	// 向 master 询问存储 filename 的位置，准备下载
	ctx, mCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer mCancel()
	req := api.FileLocationRequest{
		Filename: filename,
	}
	resp, err := client.GetFileLocation(ctx, &req)
	if err != nil {
		log.Fatalf("文件：%s 不存在", filename)
	}
	return resp.Address
}

func downloadInStream(client api.VolumeServiceClient, filename string) error {
	// 发送一个 request，返回一个流
	stream, err := client.DownloadFile(context.Background(), &api.DownloadRequest{
		Filename: filename,
	})
	if err != nil {
		log.Fatalf("发起下载失败 %v", err)
	}
	// 准备接收并写入本地
	outFile, err := os.Create("downloaded_" + filename)
	if err != nil {
		log.Fatalf("创建文件失败")
	}
	defer outFile.Close()
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break // 下载完成
		}
		if err != nil {
			log.Fatalf("接收终端 %v", err)
		}
		// 写到本地文件
		outFile.Write(resp.Content)
	}
	log.Println("下载并保存成功")
	return nil
}

func main() {
	// 问路：联系 Master
	// 建议：地址也可以通过 flag 传入
	masterConn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接 Master: %v", err)
	}
	defer masterConn.Close()

	mClient := api.NewMasterServiceClient(masterConn)

	// 设置超时并立即执行
	mCtx, mCancel := context.WithTimeout(context.Background(), time.Second*5)
	respMaster, err := mClient.AssignVolume(mCtx, &api.AssignVolumeRequest{
		Filename: "Makefile",
		FileSize: 100,
	})
	mCancel()
	if err != nil {
		log.Fatalf("Master 分配节点失败: %v", err)
	}

	log.Printf("【Client】Master 指派节点: %s", respMaster.Address)

	// 联系对应的 Volume
	// 现在的 volume 是名单列表
	vConn, err := grpc.NewClient(respMaster.Address[0], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接 Volume 节点 %s: %v", respMaster.Address, err)
	}
	defer vConn.Close()
	vClient := api.NewVolumeServiceClient(vConn)
	uploadInStream(vClient, "Makefile", "./Makefile", respMaster.Address)
	// 下载文件，先请求路径
	addr := getFileLocation(mClient, "Makefile")
	// 拿到路径以后，要进行流式读取
	// 和 volume 建立连接
	DownConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("无法连接下载节点 %s", addr)
	}
	DownClient := api.NewVolumeServiceClient(DownConn)
	downloadInStream(DownClient, "Makefile")
}
