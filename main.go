package main

import (
    "encoding/json"
    "log"
    "fmt"
    "os"
    "net/http"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"

    "io"      
    "context" 

    "strconv"

    "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
    
)

// ตัวแปรเก็บการเชื่อมต่อ Database
var db *gorm.DB

// กำหนด OAuth2 Config สำหรับ Google
var googleOauthConfig = &oauth2.Config{
    ClientID:     "myClientId",
    ClientSecret: "myClientSecret",
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

// ฟังก์ชันอัปโหลดรูปไป Google Drive
func CreateReview(c *fiber.Ctx) error {
    restroomID, err := strconv.Atoi(c.FormValue("restroom_id"))
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid restroom ID"})
    }

    userID, err := strconv.Atoi(c.FormValue("user_id"))
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
    }

    rating, err := strconv.ParseFloat(c.FormValue("rating"), 64)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid rating"})
    }

    comment := c.FormValue("comment")

    fmt.Println("🔹 Received Data - RestroomID:", restroomID, "UserID:", userID, "Rating:", rating, "Comment:", comment)

    // ✅ บันทึกรีวิวลงฐานข้อมูลก่อน
    review := Review{
        RestroomID: uint(restroomID),
        UserID:     uint(userID),
        Rating:     rating,
        Comment:    comment,
    }

    result := db.Create(&review)
    if result.Error != nil {
        fmt.Println("❌ ERROR: Failed to insert review into database:", result.Error)
        return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save review"})
    }

    fmt.Println("✅ Review successfully saved! Review ID:", review.ReviewID)

    // ✅ ลองรับไฟล์รูป (ถ้ามี)
    file, err := c.FormFile("photo")
    if err != nil {
        fmt.Println("⚠️ No image uploaded, skipping file upload")
        return c.JSON(fiber.Map{
            "message":   "Review added without image!",
            "review_id": review.ReviewID,
        })
    }

    fileData, err := file.Open()
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Cannot open file"})
    }
    defer fileData.Close()

    // ✅ อัปโหลดรูปไป Google Drive
    driveLink, err := UploadFileToDrive(file.Filename, fileData, "1P4Jks1kHKduS3yg7mk2uBXqd6EGEmPtI")
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Google Drive upload failed"})
    }

    // ✅ บันทึกรูปภาพที่เกี่ยวข้องกับรีวิว
    photo := Photo{
        Base64:        driveLink,
        PhotoRestroom: nil, // ✅ ต้องเป็น NULL สำหรับรูปคอมเมนต์
        PhotoReview:   &review.ReviewID,
    }
    db.Create(&photo)

    return c.JSON(fiber.Map{
        "message":   "Review added!",
        "review_id": review.ReviewID,
        "photo_url": driveLink,
    })
}



func UploadFileToDrive(filename string, fileData io.Reader, folderID string) (string, error) {
    ctx := context.Background()
    service, err := drive.NewService(ctx, option.WithCredentialsFile("credentials.json"))
    if err != nil {
        fmt.Println("❌ ERROR: Cannot create Google Drive service:", err)
        return "", fmt.Errorf("Google Drive service failed: %v", err)
    }

    fileMetadata := &drive.File{
        Name:    filename,
        Parents: []string{folderID},
    }

    file, err := service.Files.Create(fileMetadata).Media(fileData).Do()
    if err != nil {
        fmt.Println("❌ ERROR: Cannot upload file:", err)
        return "", fmt.Errorf("Google Drive upload failed: %v", err)
    }

    _, err = service.Permissions.Create(file.Id, &drive.Permission{
        Role: "reader", Type: "anyone",
    }).Do()
    if err != nil {
        fmt.Println("❌ ERROR: Cannot set file permission:", err)
        return "", fmt.Errorf("Google Drive permission failed: %v", err)
    }

    link := "https://drive.google.com/thumbnail?id=" + file.Id + "&sz=w1000"
    fmt.Println("✅ SUCCESS: File uploaded:", link)
    return link, nil
}







func main() {

    log.SetFlags(log.LstdFlags | log.Lshortfile)
    log.SetOutput(os.Stdout) // ✅ บังคับให้ Log ออกไป stdout

    // เชื่อมต่อฐานข้อมูล
    initDatabase()

    app := fiber.New()
    app.Use(cors.New(cors.Config{
        AllowOrigins: "*",
        AllowMethods: "GET, POST, PUT, DELETE",
    }))
    log.Println("Test TTTTT")
    fmt.Println("Test From FMTTTTT")

    // ✅ เพิ่ม API สำหรับ Google Login
    app.Post("/auth/google", googleAuthHandler)
    
    // ✅ API สำหรับดึงข้อมูลห้องน้ำ
    app.Get("/restrooms/details", func(c *fiber.Ctx) error {
        log.Println("🔹 API /restrooms/details ถูกเรียกใช้งานแล้ว")
        fmt.Println("🔹 API /restrooms/details ถูกเรียกใช้งานแล้ว FMTTTT")
        
    
        var restrooms []Restroom
        result := db.Find(&restrooms)
        if result.Error != nil {
            log.Println("❌ ดึงข้อมูลห้องน้ำไม่สำเร็จ:", result.Error)
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
                "error": "Failed to fetch restrooms",
            })
        }
    
        var restroomWithDetails []fiber.Map
    
        for _, restroom := range restrooms {
            log.Println("🚽 กำลังประมวลผลห้องน้ำ:", restroom.BuildingName)
    
            var restroomPhotos []Photo
            db.Where("photo_restroom = ?", restroom.RestroomId).Find(&restroomPhotos)
    
            var reviews []Review
            db.Where("restroom_id = ?", restroom.RestroomId).Find(&reviews)
    
            var reviewsWithPhotos []fiber.Map
            for _, review := range reviews {
                log.Println("📝 กำลังดึงข้อมูลรีวิว:", review.ReviewID, "ของ User ID:", review.UserID)
    
                var reviewPhotos []Photo
                db.Where("photo_review = ?", review.ReviewID).Find(&reviewPhotos)
    
                // ✅ ตรวจสอบว่ามี User จริงหรือไม่
                var user User
                userQuery := db.Where("user_id = ?", review.UserID).First(&user)
    
                firstName := "Unknown"
                lastName := "User"
                if userQuery.Error == nil {
                    firstName = user.FirstName
                    lastName = user.LastName
                } else {
                    log.Println("⚠️ ไม่พบข้อมูล User ID:", review.UserID, "| Error:", userQuery.Error)
                }
    
                fmt.Println("✅ Review ID:", review.ReviewID, "User:", firstName, lastName) // Debugging ชื่อ User
    
                reviewsWithPhotos = append(reviewsWithPhotos, fiber.Map{
                    "review": fiber.Map{
                        "review_id":  review.ReviewID,
                        "restroom_id": review.RestroomID,
                        "user_id":     review.UserID,
                        "first_name":  firstName, // ✅ เพิ่มชื่อที่นี่
                        "last_name":   lastName,  // ✅ เพิ่มนามสกุลที่นี่
                        "rating":      review.Rating,
                        "comment":     review.Comment,
                    },
                    "photos": reviewPhotos,
                })
            }
    
            restroomWithDetails = append(restroomWithDetails, fiber.Map{
                "restroom":        restroom,
                "restroom_photos": restroomPhotos,
                "reviews":         reviewsWithPhotos,
            })
        }
    
        log.Println("✅ API /restrooms/details ส่งข้อมูลสำเร็จ")
        return c.JSON(restroomWithDetails)
    })
    
    
    
    app.Post("/review", CreateReview) // ✅ API สร้างรีวิวพร้อมรูป
    



    
    
    
    
    
    log.Fatal(app.Listen(":3001"))
}
