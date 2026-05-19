package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/config"
	"github.com/mesnet/mesnet/internal/server/database"
	"github.com/mesnet/mesnet/internal/server/handlers"
	"github.com/mesnet/mesnet/internal/server/middleware"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/services"
	"github.com/mesnet/mesnet/internal/server/ws"
	"github.com/mesnet/mesnet/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Current)
		return
	}
	cfg := config.Load()

	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer database.Close(db)

	database.Migrate(db)

	// Seed default admin if no users exist
	handlers.SeedAdmin(db)

	// Generate or use JWT secret
	jwtSecret := make([]byte, 32)
	if s := os.Getenv("JWT_SECRET"); s != "" {
		copy(jwtSecret, []byte(s))
	} else {
		rand.Read(jwtSecret)
	}
	middleware.SetJWTSecret(jwtSecret)

	registry := ws.NewRegistry()
	go registry.Run()

	// Auto-mesh when agent connects — wait 3s then mesh ALL online nodes
	registry.SetOnHello(func(nodeID uint) {
		go func() {
			time.Sleep(3 * time.Second)
			for _, id := range registry.ListOnline() {
				services.AutoMesh(db, registry, id)
			}
		}()
	})

	// Notify peers when agent disconnects for fast convergence
	registry.SetOnUnregister(func(nodeID uint) {
		go func() {
			// Mark tunnels as down
			db.Model(&models.Tunnel{}).Where("left_node_id = ? OR right_node_id = ?", nodeID, nodeID).
				Update("status", "down")
			// Notify other peers to clean up
			for _, id := range registry.ListOnline() {
				registry.SendCmd(id, "peer_disconnect", map[string]any{"peer_id": nodeID}, 3*time.Second)
			}
			// Immediate failover: switch leaves that used this backbone
			var leaves []models.Node
			db.Where("backbone = ?", false).Find(&leaves)
			for _, leaf := range leaves {
				if !registry.IsOnline(leaf.ID) {
					continue
				}
				var tunnel models.Tunnel
				if db.Where(
					"(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
					leaf.ID, nodeID, nodeID, leaf.ID,
				).First(&tunnel).Error == nil {
					log.Printf("failover: backbone %d offline, immediately switching leaf %d", nodeID, leaf.ID)
					services.SwitchBackbone(db, registry, &leaf, nodeID)
				}
			}
		}()
	})

	// Start stats collector
	services.StartCollector(db, registry)

	// Start failover for leaf nodes
	go services.CheckAndFailover(db, registry)

	r := gin.Default()

	r.Use(corsMiddleware())
	r.Use(gin.Logger())

	// Static files (React build)
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/favicon.ico", "./web/dist/favicon.ico")

	// WebSocket agent endpoint
	r.GET("/ws/agent/:token", func(c *gin.Context) {
		ws.HandleAgent(c.Writer, c.Request, registry, db)
	})

	// Agent binary download
	r.GET("/api/agent/binary", func(c *gin.Context) {
		c.File("./mesnet-agent")
	})

	// SPA fallback
	r.NoRoute(func(c *gin.Context) {
		c.File("./web/dist/index.html")
	})

	// REST API
	api := r.Group("/api")
	api.Use(middleware.AuthRequired("/api/auth/login"))
	{
		// Auth (login is public, others require auth)
		api.POST("/auth/login", handlers.Login(db))
		api.POST("/auth/change-password", handlers.ChangePassword(db))
		api.GET("/auth/me", handlers.Me(db))

		api.GET("/stats", handlers.GetDashboardStats(db, registry))
		api.GET("/topology", handlers.GetTopology(db, registry))
		api.GET("/audit", handlers.ListAudit(db))
		api.GET("/logs", handlers.GetLogs())
		api.GET("/monitor/total", handlers.GetTotalTraffic(db))

		// Server management
		api.GET("/servers", handlers.GetServers(db, registry))
		api.POST("/servers/cloud", handlers.AddCloudServer(db, registry))
		api.POST("/servers/leaf", handlers.AddLeafNode(db, registry))
		api.GET("/servers/:id/deploy", handlers.GetServerDeploy(db))
		api.POST("/servers/:id/auto-deploy", handlers.AutoDeployNode(db))
		api.POST("/servers/test-ssh", handlers.TestSSH())

		// Agent updates
		api.GET("/agents/versions", handlers.GetAgentVersions(db, registry))
		api.POST("/agents/update", handlers.UpdateAgent(registry))
		api.POST("/agents/update-all", handlers.UpdateAllAgents(registry))
		api.POST("/server/update", handlers.UpdateServer())

		nodes := api.Group("/nodes")
		{
			nodes.GET("", handlers.ListNodes(db, registry))
			nodes.GET("/:id", handlers.GetNode(db, registry))
			nodes.POST("", handlers.CreateNode(db))
			nodes.PUT("/:id", handlers.UpdateNode(db))
			nodes.DELETE("/:id", handlers.DeleteNode(db))
				nodes.POST("/:id/detect-subnets", handlers.DetectSubnets(db, registry))
				nodes.POST("/:id/tunnel-test", handlers.TestTunnel(db, registry))
			nodes.GET("/:id/deploy", handlers.GetDeployScript(db, cfg))
			nodes.GET("/:id/stats", handlers.GetNodeStats(db, registry))
		}

		tunnels := api.Group("/tunnels")
		{
			tunnels.GET("", handlers.ListTunnels(db))
			tunnels.GET("/:id", handlers.GetTunnel(db))
			tunnels.POST("", handlers.CreateTunnel(db, registry))
			tunnels.DELETE("/:id", handlers.DeleteTunnel(db, registry))
			tunnels.POST("/:id/up", handlers.TunnelUp(db, registry))
			tunnels.POST("/:id/down", handlers.TunnelDown(db, registry))
			tunnels.GET("/:id/stats", handlers.GetTunnelStats(db))
			tunnels.GET("/:id/stats/history", handlers.GetTunnelStatsHistory(db))
		}
	}

	go func() {
		if err := r.Run(":8080"); err != nil {
			log.Fatalf("server start failed: %v", err)
		}
	}()

	log.Println("MeshNet control plane started on :8080")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
