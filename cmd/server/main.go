package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/config"
	"github.com/mesnet/mesnet/internal/server/database"
	"github.com/mesnet/mesnet/internal/server/handlers"
	"github.com/mesnet/mesnet/internal/server/services"
	"github.com/mesnet/mesnet/internal/server/ws"
)

func main() {
	cfg := config.Load()

	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer database.Close(db)

	database.Migrate(db)

	registry := ws.NewRegistry()
	go registry.Run()

	// Auto-mesh when agent connects
	registry.SetOnHello(func(nodeID uint) {
		services.AutoMesh(db, registry, nodeID)
	})

	// Start stats collector
	services.StartCollector(db, registry)

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
	{
		api.GET("/stats", handlers.GetDashboardStats(db, registry))
		api.GET("/topology", handlers.GetTopology(db, registry))
		api.GET("/audit", handlers.ListAudit(db))
		api.GET("/monitor/total", handlers.GetTotalTraffic(db))

		// Server management
		api.GET("/servers", handlers.GetServers(db, registry))
		api.POST("/servers/cloud", handlers.AddCloudServer(db, registry))
		api.POST("/servers/leaf", handlers.AddLeafNode(db, registry))
		api.GET("/servers/:id/deploy", handlers.GetServerDeploy(db))

		// Agent updates
		api.GET("/agents/versions", handlers.GetAgentVersions(db, registry))
		api.POST("/agents/update", handlers.UpdateAgent(registry))
		api.POST("/agents/update-all", handlers.UpdateAllAgents(registry))

		nodes := api.Group("/nodes")
		{
			nodes.GET("", handlers.ListNodes(db, registry))
			nodes.GET("/:id", handlers.GetNode(db, registry))
			nodes.POST("", handlers.CreateNode(db))
			nodes.PUT("/:id", handlers.UpdateNode(db))
			nodes.DELETE("/:id", handlers.DeleteNode(db))
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
