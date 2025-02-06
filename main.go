package main

import (
    "log"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "os"
)

// ตัวแปรเก็บการเชื่อมต่อ DB
var db *gorm.DB

// โครงสร้าง Model
type Message struct {
    ID      uint   `json:"id" gorm:"primaryKey"`
    Content string `json:"content"`
}

// ฟังก์ชันเชื่อมต่อกับ PostgreSQL
func initDatabase() {
    var err error
    dsn := "host=localhost user=postgres password=peempleng123 dbname=ku-toilet port=5432 sslmode=disable"
    db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("❌ Failed to connect to database: %v", err)
    }

    // สร้างตารางอัตโนมัติ
    db.AutoMigrate(&Message{})
    log.Println("✅ Database connected and migrated!")
}

func main() {
    // เชื่อมต่อฐานข้อมูล
    initDatabase()

    app := fiber.New()

    // CORS Middleware (อนุญาตทุก Origin)
    app.Use(cors.New(cors.Config{
        AllowOrigins: "*",
        AllowMethods: "GET,POST,DELETE",
    }))

    // API ทดสอบ
    app.Get("/test", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{"message": "Hello from Backend"})
    })

    // API ดึงข้อความจาก Database
    app.Get("/messages", func(c *fiber.Ctx) error {
        var messages []Message
        db.Find(&messages)
        return c.JSON(messages)
    })

    // API บันทึกข้อความ
    app.Post("/messages", func(c *fiber.Ctx) error {
        var msg Message
        if err := c.BodyParser(&msg); err != nil {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
        }

        if msg.Content == "" {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Message content cannot be empty"})
        }

        db.Create(&msg)
        return c.Status(fiber.StatusOK).JSON(msg)
    })

    // API ลบข้อความ
    app.Delete("/messages/:id", func(c *fiber.Ctx) error {
        id := c.Params("id")
        db.Delete(&Message{}, id)
        return c.JSON(fiber.Map{"message": "Deleted successfully"})
    })

    log.Fatal(app.Listen(":3001"))
}
