package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/seds-net/seds-agent/config"
	"github.com/seds-net/seds-agent/proto"
	"github.com/seds-net/seds-agent/singbox"
	"github.com/seds-net/seds-agent/stats"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client represents the gRPC client
type Client struct {
	conn           *grpc.ClientConn
	stream         proto.AgentService_ConnectClient
	sbManager      *singbox.Manager
	statsCollector *stats.Collector
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewClient creates a new gRPC client
func NewClient(sbManager *singbox.Manager) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		sbManager:      sbManager,
		statsCollector: stats.NewCollector(),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Connect establishes connection to the server
func (c *Client) Connect() error {
	cfg := config.Get()

	log.Printf("Connecting to server: %s", cfg.Server)

	// Dial server
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		cfg.Server,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn

	// Create service client
	client := proto.NewAgentServiceClient(conn)

	// Start bidirectional stream
	stream, err := client.Connect(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	c.stream = stream

	// Send registration message
	if err := c.register(); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	// Start message handlers
	go c.receiveMessages()
	go c.sendHeartbeat()

	log.Println("Connected and registered successfully")
	return nil
}

// register sends registration message to server
func (c *Client) register() error {
	cfg := config.Get()

	msg := &proto.AgentMessage{
		Payload: &proto.AgentMessage_Register{
			Register: &proto.RegisterRequest{
				Token:   cfg.Token,
				Version: "1.0.0",
			},
		},
	}

	return c.stream.Send(msg)
}

// sendHeartbeat periodically sends heartbeat messages
func (c *Client) sendHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// Collect system stats
			sysStats, err := c.statsCollector.Collect()
			if err != nil {
				log.Printf("Warning: failed to collect stats: %v", err)
			}

			// Send heartbeat with system stats
			msg := &proto.AgentMessage{
				Payload: &proto.AgentMessage_Heartbeat{
					Heartbeat: &proto.Heartbeat{
						Timestamp: time.Now().Unix(),
					},
				},
			}

			if err := c.stream.Send(msg); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
				c.cancel() // Trigger reconnection
				return
			}

			// Send system stats
			if sysStats != nil {
				var statsMap map[string]interface{}
				json.Unmarshal(sysStats, &statsMap)

				statsMsg := &proto.AgentMessage{
					Payload: &proto.AgentMessage_SysStats{
						SysStats: &proto.SysStats{
							Cpu:     getStringField(statsMap, "cpu"),
							Memory:  getStringField(statsMap, "memory"),
							Disk:    getStringField(statsMap, "disk"),
							Network: getStringField(statsMap, "network"),
							Uptime:  getStringField(statsMap, "uptime"),
						},
					},
				}

				if err := c.stream.Send(statsMsg); err != nil {
					log.Printf("Failed to send stats: %v", err)
				}
			}

			// Send sing-box status
			sbStatus := c.sbManager.GetStatus()
			statusMsg := &proto.AgentMessage{
				Payload: &proto.AgentMessage_SbStatus{
					SbStatus: &proto.SbStatus{
						Running:     sbStatus.Running,
						Connections: 0, // TODO: Implement connection tracking
						Upload:      0, // TODO: Implement traffic tracking
						Download:    0, // TODO: Implement traffic tracking
					},
				},
			}

			if err := c.stream.Send(statusMsg); err != nil {
				log.Printf("Failed to send sing-box status: %v", err)
			}
		}
	}
}

// receiveMessages receives messages from the server
func (c *Client) receiveMessages() {
	for {
		msg, err := c.stream.Recv()
		if err != nil {
			log.Printf("Stream receive error: %v", err)
			c.cancel() // Trigger reconnection
			return
		}

		if err := c.handleMessage(msg); err != nil {
			log.Printf("Error handling message: %v", err)
		}
	}
}

// handleMessage processes received server messages
func (c *Client) handleMessage(msg *proto.ServerMessage) error {
	switch payload := msg.Payload.(type) {
	case *proto.ServerMessage_RegisterResponse:
		return c.handleRegisterResponse(payload.RegisterResponse)
	case *proto.ServerMessage_PushConfig:
		return c.handlePushConfig(payload.PushConfig)
	case *proto.ServerMessage_Command:
		return c.handleCommand(payload.Command)
	default:
		log.Printf("Unknown message type received")
	}
	return nil
}

// handleRegisterResponse processes registration response
func (c *Client) handleRegisterResponse(resp *proto.RegisterResponse) error {
	if resp.Success {
		log.Printf("Registration successful: %s (Node ID: %d)", resp.Message, resp.NodeId)
	} else {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}
	return nil
}

// handlePushConfig processes configuration push from server
func (c *Client) handlePushConfig(config *proto.PushConfig) error {
	log.Printf("Received configuration (version: %d)", config.Version)

	// Update sing-box configuration
	if err := c.sbManager.UpdateConfig([]byte(config.ConfigJson)); err != nil {
		log.Printf("Failed to update config: %v", err)
		return err
	}

	// Restart or start sing-box
	if c.sbManager.IsRunning() {
		log.Println("Restarting sing-box with new configuration...")
		if err := c.sbManager.Restart(); err != nil {
			log.Printf("Failed to restart sing-box: %v", err)
			return err
		}
	} else {
		log.Println("Starting sing-box with new configuration...")
		if err := c.sbManager.Start(); err != nil {
			log.Printf("Failed to start sing-box: %v", err)
			return err
		}
	}

	log.Println("Configuration applied successfully")
	return nil
}

// handleCommand processes remote commands
func (c *Client) handleCommand(cmd *proto.Command) error {
	log.Printf("Received command: %s (ID: %s)", cmd.Type, cmd.CommandId)

	var err error
	var output string

	switch cmd.Type {
	case "start":
		err = c.sbManager.Start()
		output = "Sing-box started"
	case "stop":
		err = c.sbManager.Stop()
		output = "Sing-box stopped"
	case "restart":
		err = c.sbManager.Restart()
		output = "Sing-box restarted"
	case "status":
		status := c.sbManager.GetStatus()
		statusJSON, _ := json.Marshal(status)
		output = string(statusJSON)
	default:
		err = fmt.Errorf("unknown command: %s", cmd.Type)
	}

	// Send command result
	result := &proto.AgentMessage{
		Payload: &proto.AgentMessage_CommandResult{
			CommandResult: &proto.CommandResult{
				CommandId: cmd.CommandId,
				Success:   err == nil,
				Output:    output,
			},
		},
	}

	if err != nil {
		result.GetCommandResult().Error = err.Error()
	}

	return c.stream.Send(result)
}

// Close closes the connection
func (c *Client) Close() error {
	c.cancel()
	if c.stream != nil {
		c.stream.CloseSend()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Run runs the client with auto-reconnection
func (c *Client) Run() {
	for {
		if err := c.Connect(); err != nil {
			log.Printf("Connection failed: %v", err)
			log.Println("Retrying in 10 seconds...")
			time.Sleep(10 * time.Second)
			continue
		}

		// Wait for disconnection
		<-c.ctx.Done()

		log.Println("Disconnected. Reconnecting in 5 seconds...")
		time.Sleep(5 * time.Second)

		// Reset context for next connection
		c.ctx, c.cancel = context.WithCancel(context.Background())
	}
}

// Helper function to extract string field from map
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		// Try to marshal it to JSON if it's a complex object
		if bytes, err := json.Marshal(val); err == nil {
			return string(bytes)
		}
	}
	return ""
}
