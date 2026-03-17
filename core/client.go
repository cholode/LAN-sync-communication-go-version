package core

import (
	//"bytes"
	"github.com/gorilla/websocket"
	//"os"
	//"github.com/joho/godotenv"
	"log"
	"time"
)

const (
	// 面试考点：必须设置超时机制，否则半开连接(死连接)会耗尽服务器文件描述符
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096 // 限制单条消息最大体积，防止恶意发大包打爆内存
)

type Client struct {
	Hub    *Hub
	UserID int64
	Conn   *websocket.Conn

	Send chan []byte // 专门用于下发消息的管道
}

// ReadPump 负责监听客户端发来的消息
func (c *Client) ReadPump() {
	defer func() {
		// 客户端断开或异常时，执行清理逻辑
		// c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	// 心跳机制：收到 Pong 响应时，重置读超时时间
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("异常断开: %v", err)
			}
			break
		}

		// 简单的去除两端空格处理
		//message = bytes.TrimSpace(bytes.Replace(message, []byte{'\n'}, []byte{' '}, -1))

		// 收到消息后，理论上应该解析 JSON (你之前定义的 payload)，
		// 然后判断是存库还是转发。这里为了演示，直接投递给 Hub 的广播 channel
		// c.Hub.Broadcast <- message
		log.Printf("收到消息: %s", message)
	}
}

// WritePump 负责将服务端的数据下发给客户端
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			// 监听业务层需要下发的消息
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Send channel 被关闭，服务端主动断开连接
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量写入：如果通道里还有排队的消息，一并写进去，提升网络吞吐量
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			// 定时触发心跳探测
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // 心跳发送失败，通常是因为客户端掉线，直接退出 Goroutine
			}
		}
	}
}
