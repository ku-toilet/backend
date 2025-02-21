package main

import (
    "log"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"

)

// ตัวแปรเก็บการเชื่อมต่อ Database
var db *gorm.DB

// โครงสร้าง Model
type Restroom struct {
    RestroomId           uint    `json:"restroom_id" gorm:"primaryKey;autoIncrement"`
    BuildingName         string `json:"building_name" gorm:"not null"`
    Floor               int    `json:"floor" gorm:"not null"`
    IsMen               bool   `json:"is_men" gorm:"not null"`
    IsWomen             bool   `json:"is_women" gorm:"not null"`
    IsAccessible        bool   `json:"is_accessible" gorm:"not null"`
    IsBumGun            bool   `json:"is_bum_gun" gorm:"not null"`
    IsToiletPaper       bool   `json:"is_toilet_paper" gorm:"not null"`
    IsFree              bool   `json:"is_free" gorm:"not null"`
    Latitude            string `json:"latitude" gorm:"not null"`
    Longitude           string `json:"longitude" gorm:"not null"`
    FacultyName         string `json:"faculty_name"`
    OpeningHoursMonday  string `json:"opening_hours_monday"`
    OpeningHoursTuesday string `json:"opening_hours_tuesday"`
    OpeningHoursWednesday string `json:"opening_hours_wednesday"`
    OpeningHoursThursday  string `json:"opening_hours_thursday"`
    OpeningHoursFriday    string `json:"opening_hours_friday"`
    OpeningHoursSaturday  string `json:"opening_hours_saturday"`
    OpeningHoursSunday    string `json:"opening_hours_sunday"`
}

type Review struct {
    ReviewID   uint    `json:"review_id" gorm:"primaryKey;autoIncrement"`
    RestroomID uint    `json:"restroom_id" gorm:"not null"` // Foreign Key ไปยัง Restroom
    UserID     uint    `json:"user_id" gorm:"not null"`     // เก็บ ID ของผู้ใช้ที่รีวิว
    Rating     float64 `json:"rating" gorm:"not null"`      // คะแนนรีวิว (float)
    Comment    string  `json:"comment"`                    // ข้อความรีวิว
}


// ตาราง Photo
type Photo struct {
    PhotoID       uint    `json:"photo_id" gorm:"primaryKey;autoIncrement"`
    Base64        string  `json:"base64" gorm:"not null"`
    PhotoRestroom *uint   `json:"photo_restroom" gorm:"default:null"`
    PhotoReview   *uint   `json:"photo_review" gorm:"default:null"`
}





// ฟังก์ชันเชื่อมต่อกับ Database
func initDatabase() {
    var err error
    dsn := "host=postgres user=postgres password=peempleng123 dbname=ku-toilet port=5432 sslmode=disable"
    db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("❌ Failed to connect to database: %v", err)
    }

     // ✅ สร้างตารางทั้งหมด
     db.AutoMigrate(&Restroom{}, &Review{}, &Photo{}) 
     log.Println("✅ Database connected and migrated!")
 
}

func main() {
    // เชื่อมต่อฐานข้อมูล
    initDatabase()

    app := fiber.New()

    // อนุญาตให้ Frontend ดึง API ได้
    app.Use(cors.New(cors.Config{
        AllowOrigins: "*",
        AllowMethods: "GET,POST",
    }))

    // API ดึงข้อมูลห้องน้ำทั้งหมด
    app.Get("/restrooms", func(c *fiber.Ctx) error {
        var restrooms []Restroom
        db.Find(&restrooms)
        return c.JSON(restrooms)
    })

    app.Get("/", func(c *fiber.Ctx) error {
        return c.SendString("Backend is running!")
    })    


     // API ดึงข้อมูลห้องน้ำทั้งหมด รูปภาพและรีวิว
    app.Get("/restrooms/details", func(c *fiber.Ctx) error {
        var restrooms []Restroom
        db.Preload("Photos").Find(&restrooms) // ✅ ใช้ Preload โหลดภาพ
    
        var restroomWithDetails []fiber.Map
    
        for _, restroom := range restrooms {
            // 🔹 ดึงรูปภาพของห้องน้ำ
            var restroomPhotos []Photo
            db.Where("photo_restroom = ?", restroom.RestroomId).Find(&restroomPhotos)
    
            // 🔹 ดึงรีวิวของห้องน้ำ
            var reviews []Review
            db.Where("restroom_id = ?", restroom.RestroomId).Find(&reviews)
    
            // 🔹 ดึงรูปภาพที่เกี่ยวข้องกับแต่ละรีวิว
            var reviewsWithPhotos []fiber.Map
            for _, review := range reviews {
                var reviewPhotos []Photo
                db.Where("photo_review = ?", review.ReviewID).Find(&reviewPhotos)
    
                reviewsWithPhotos = append(reviewsWithPhotos, fiber.Map{
                    "review": review,
                    "photos": reviewPhotos,
                })
            }
    
            restroomWithDetails = append(restroomWithDetails, fiber.Map{
                "restroom":        restroom,
                "restroom_photos": restroomPhotos,
                "reviews":         reviewsWithPhotos, // ✅ ถ้าไม่มีรีวิวให้คืนค่า []
            })
        }
    
        return c.JSON(restroomWithDetails)
    })
    
    
    
    
    log.Fatal(app.Listen(":3001"))
}
