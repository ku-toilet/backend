package main

import (
    "encoding/json"
    "log"
    "net/http"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    
)

// ตัวแปรเก็บการเชื่อมต่อ Database
var db *gorm.DB

// กำหนด OAuth2 Config สำหรับ Google
var googleOauthConfig = &oauth2.Config{
    ClientID:     "clinet id",
    ClientSecret: "googlesecret",
    RedirectURL:  "http://localhost:3000",
    Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
    Endpoint:     google.Endpoint,
}

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

// ตาราง Review
type Review struct {
    ReviewID   uint    `json:"review_id" gorm:"primaryKey;autoIncrement"`
    RestroomID uint    `json:"restroom_id" gorm:"not null"`
    UserID     uint    `json:"user_id" gorm:"not null"`
    Rating     float64 `json:"rating" gorm:"not null"`
    Comment    string  `json:"comment"`
}


// ตาราง Photo
type Photo struct {
    PhotoID       uint    `json:"photo_id" gorm:"primaryKey;autoIncrement"`
    Base64        string  `json:"base64" gorm:"not null"`
    PhotoRestroom *uint   `json:"photo_restroom" gorm:"default:null"`
    PhotoReview   *uint   `json:"photo_review" gorm:"default:null"`
}

// ตาราง User
type User struct {
    UserId    uint   `json:"user_id" gorm:"primaryKey;autoIncrement"`
    FirstName string `json:"first_name" gorm:"not null"`
    LastName  string `json:"last_name" gorm:"not null"`
    Email     string `json:"email" gorm:"unique;not null"`
    Role      string `json:"role" gorm:"not null;default:'user'"`
}

// ฟังก์ชันเชื่อมต่อกับ Database
func initDatabase() {
    var err error
    dsn := "host=postgres user=postgres password=peempleng123 dbname=ku-toilet port=5432 sslmode=disable"
    db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("❌ Failed to connect to database: %v", err)
    }
    db.AutoMigrate(&Restroom{}, &Review{}, &Photo{}, &User{}) 
    log.Println("✅ Database connected and migrated!")
}

// ✅ API รับ Token จาก Frontend และตรวจสอบกับ Google
func googleAuthHandler(c *fiber.Ctx) error {
    var requestData struct {
        Token string `json:"token"`
    }
    if err := c.BodyParser(&requestData); err != nil {
        return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
    }
    tokenInfoURL := "https://www.googleapis.com/oauth2/v3/tokeninfo?id_token=" + requestData.Token
    resp, err := http.Get(tokenInfoURL)
    if err != nil || resp.StatusCode != http.StatusOK {
        return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
    }
    var tokenData struct {
        Email     string `json:"email"`
        GivenName string `json:"given_name"`
        FamilyName string `json:"family_name"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
        return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse token data"})
    }
    var user User
    result := db.Where("email = ?", tokenData.Email).First(&user)
    if result.Error != nil {
        user = User{
            FirstName: tokenData.GivenName,
            LastName:  tokenData.FamilyName,
            Email:     tokenData.Email,
        }
        db.Create(&user)
    }
    return c.JSON(fiber.Map{
        "message": "User authenticated",
        "user":    user,
    })
}

func main() {
    // เชื่อมต่อฐานข้อมูล
    initDatabase()

    app := fiber.New()
    app.Use(cors.New(cors.Config{ AllowOrigins: "*", AllowMethods: "GET,POST" }))
    
    // ✅ เพิ่ม API สำหรับ Google Login
    app.Post("/auth/google", googleAuthHandler)
    
    // ✅ API สำหรับดึงข้อมูลห้องน้ำ
    app.Get("/restrooms", func(c *fiber.Ctx) error {
        var restrooms []Restroom
        db.Find(&restrooms)
        return c.JSON(restrooms)
    })
    
    app.Get("/", func(c *fiber.Ctx) error {
        return c.SendString("Backend is running!")
    })
    
    app.Get("/restrooms/details", func(c *fiber.Ctx) error {
        var restrooms []Restroom
        db.Find(&restrooms)
    
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
                "reviews":         reviewsWithPhotos,
            })
        }
    
        return c.JSON(restroomWithDetails)
    })
    
    
    
    
    log.Fatal(app.Listen(":3001"))
}
