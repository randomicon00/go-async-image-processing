package main

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"pool_enhanced/workerpool"
)

// ImageJob represents a single image processing job
type ImageJob struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	Status     string    `json:"status"` // pending, processing, completed, failed
	InputPath  string    `json:"input_path"`
	OutputPath string    `json:"output_path"`
	Operation  string    `json:"operation"` // resize, grayscale, etc.
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SubmitRequest represents a job submission request
type SubmitRequest struct {
	ImageData    string `json:"image_data" binding:"required"` // base64 encoded
	Operation    string `json:"operation" binding:"required,oneof=resize thumbnail"`
	Width        int    `json:"width" binding:"required,min=1,max=4096"`
	Height       int    `json:"height" binding:"required,min=1,max=4096"`
	OutputFormat string `json:"output_format" binding:"omitempty,oneof=jpeg png"`
}

// Config holds application configuration
type Config struct {
	Port           string
	DBPath         string
	UploadDir      string
	OutputDir      string
	WorkerPoolSize int
	NumPools       int
	MaxUploadSize  int64
}

var (
	db     *gorm.DB
	wp     *workerpool.Pool
	config Config
)

func main() {
	// Load configuration
	loadConfig()

	// Setup directories
	setupDirectories()

	// Initialize database
	initDB()

	// Initialize worker pool
	wp = workerpool.NewPool(config.WorkerPoolSize)
	wp.Start()
	defer wp.Stop()

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggingMiddleware())

	// Routes
	r.POST("/jobs", submitJobHandler)
	r.GET("/jobs/:id", getJobStatusHandler)
	r.GET("/jobs/:id/download", downloadImageHandler)
	r.GET("/health", healthCheckHandler)

	// Setup HTTP server with graceful shutdown
	srv := &http.Server{
		Addr:    ":" + config.Port,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on port %s", config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Wait for all jobs to complete
	wp.Wait()
	log.Println("Server gracefully stopped")
}

func loadConfig() {
	config.Port = getEnv("PORT", "8080")
	config.DBPath = getEnv("DB_PATH", "./data/jobs.db")
	config.UploadDir = getEnv("UPLOAD_DIR", "./uploads")
	config.OutputDir = getEnv("OUTPUT_DIR", "./outputs")
	config.WorkerPoolSize = getEnvInt("WORKER_POOL_SIZE", 10)
	config.NumPools = getEnvInt("NUM_POOLS", 2)
	config.MaxUploadSize = getEnvInt64("MAX_UPLOAD_SIZE", 10*1024*1024) // 10MB
}

func setupDirectories() {
	dirs := []string{filepath.Dir(config.DBPath), config.UploadDir, config.OutputDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}
}

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open(config.DBPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ImageJob{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
}

func submitJobHandler(c *gin.Context) {
	var req SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate job ID
	jobID := uuid.New().String()

	// Create input/output paths
	inputPath := filepath.Join(config.UploadDir, fmt.Sprintf("%s_input", jobID))
	outputPath := filepath.Join(config.OutputDir, fmt.Sprintf("%s_output", jobID))

	// Decode and save image (simplified - in real app, validate and decode properly)
	if err := os.WriteFile(inputPath, []byte(req.ImageData), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image"})
		return
	}

	// Create job record
	job := ImageJob{
		ID:         jobID,
		Status:     "pending",
		InputPath:  inputPath,
		OutputPath: outputPath,
		Operation:  req.Operation,
		Width:      req.Width,
		Height:     req.Height,
	}

	if err := db.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job"})
		return
	}

	// Submit job to worker pool
	wp.Submit(func() error {
		return processImage(&job, req.OutputFormat)
	})

	c.JSON(http.StatusAccepted, gin.H{
		"job_id":  jobID,
		"status":  "pending",
		"message": "Image processing job submitted",
	})
}

func getJobStatusHandler(c *gin.Context) {
	jobID := c.Param("id")

	var job ImageJob
	if err := db.First(&job, "id = ?", jobID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch job"})
		return
	}

	c.JSON(http.StatusOK, job)
}

func downloadImageHandler(c *gin.Context) {
	jobID := c.Param("id")

	var job ImageJob
	if err := db.First(&job, "id = ?", jobID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if job.Status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image not ready"})
		return
	}

	c.File(job.OutputPath)
}

func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
	})
}

func processImage(job *ImageJob, outputFormat string) error {
	// Update status to processing
	job.Status = "processing"
	if err := db.Save(job).Error; err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	// Open input image
	file, err := os.Open(job.InputPath)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		db.Save(job)
		return fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	// Decode image
	img, format, err := image.Decode(file)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		db.Save(job)
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Process based on operation
	var processedImg image.Image
	switch job.Operation {
	case "resize":
		processedImg = resizeImage(img, job.Width, job.Height)
	case "thumbnail":
		processedImg = createThumbnail(img, job.Width, job.Height)
	default:
		processedImg = img
	}

	// Determine output format
	if outputFormat == "" {
		outputFormat = format
	}

	// Save processed image
	outFile, err := os.Create(job.OutputPath)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		db.Save(job)
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	switch outputFormat {
	case "jpeg", "jpg":
		err = jpeg.Encode(outFile, processedImg, &jpeg.Options{Quality: 90})
	case "png":
		err = png.Encode(outFile, processedImg)
	default:
		err = jpeg.Encode(outFile, processedImg, &jpeg.Options{Quality: 90})
	}

	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		db.Save(job)
		return fmt.Errorf("failed to encode image: %w", err)
	}

	// Mark as completed
	job.Status = "completed"
	if err := db.Save(job).Error; err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	log.Printf("Job %s completed successfully", job.ID)
	return nil
}

func resizeImage(img image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

func createThumbnail(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// Calculate scaling factor to fit within max dimensions
	scaleX := float64(maxWidth) / float64(origWidth)
	scaleY := float64(maxHeight) / float64(origHeight)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	newWidth := int(float64(origWidth) * scale)
	newHeight := int(float64(origHeight) * scale)

	return resizeImage(img, newWidth, newHeight)
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		log.Printf("[%s] %s %s %d %v",
			clientIP,
			method,
			path,
			statusCode,
			latency,
		)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}
