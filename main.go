package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	
)

// ตัวแปรเก็บการเชื่อมต่อ Database
var db *gorm.DB

// กำหนด OAuth2 Config สำหรับ Google
var googleOauthConfig = &oauth2.Config{
	ClientID:     "",
	ClientSecret: "",
	RedirectURL:  "http://localhost:3000",
	Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
	Endpoint:     google.Endpoint,
}

// โครงสร้าง Model
type Restroom struct {
	RestroomId            uint   `json:"restroom_id" gorm:"primaryKey;autoIncrement"`
	BuildingName          string `json:"building_name" gorm:"not null"`
	Floor                 int    `json:"floor" gorm:"not null"`
	IsMen                 bool   `json:"is_men" gorm:"not null"`
	IsWomen               bool   `json:"is_women" gorm:"not null"`
	IsAccessible          bool   `json:"is_accessible" gorm:"not null"`
	IsBumGun              bool   `json:"is_bum_gun" gorm:"not null"`
	IsToiletPaper         bool   `json:"is_toilet_paper" gorm:"not null"`
	IsFree                bool   `json:"is_free" gorm:"not null"`
	Latitude              string `json:"latitude" gorm:"not null"`
	Longitude             string `json:"longitude" gorm:"not null"`
	FacultyName           string `json:"faculty_name"`
	OpeningHoursMonday    string `json:"opening_hours_monday"`
	OpeningHoursTuesday   string `json:"opening_hours_tuesday"`
	OpeningHoursWednesday string `json:"opening_hours_wednesday"`
	OpeningHoursThursday  string `json:"opening_hours_thursday"`
	OpeningHoursFriday    string `json:"opening_hours_friday"`
	OpeningHoursSaturday  string `json:"opening_hours_saturday"`
	OpeningHoursSunday    string `json:"opening_hours_sunday"`
}

// ตาราง Review
type Review struct {
	ReviewID   uint      `json:"review_id" gorm:"primaryKey;autoIncrement"`
	RestroomID uint      `json:"restroom_id" gorm:"not null"`
	UserID     uint      `json:"user_id" gorm:"not null"`
	Rating     float64   `json:"rating" gorm:"not null"`
	Comment    string    `json:"comment"`
	ReviewDate time.Time `json:"review_date" gorm:"type:date;default:CURRENT_DATE"` // เพิ่มฟิลด์วันที่
}

// ตาราง Photo แก้ไขให้ base64 เป็น text
type Photo struct {
	PhotoID       uint   `json:"photo_id" gorm:"primaryKey;autoIncrement"`
	Base64        string `json:"base64" gorm:"type:text;not null"` // เปลี่ยนเป็น type:text
	PhotoRestroom *uint  `json:"photo_restroom" gorm:"default:null"`
	PhotoReview   *uint  `json:"photo_review" gorm:"default:null"`
}

// ตาราง User
type User struct {
	UserId    uint   `json:"user_id" gorm:"primaryKey;autoIncrement"`
	FirstName string `json:"first_name" gorm:"not null"`
	LastName  string `json:"last_name" gorm:"not null"`
	Email     string `json:"email" gorm:"unique;not null"`
	Role      string `json:"role" gorm:"not null;default:'user'"`
}

type ReviewBase64Request struct {
	RestroomID  string `json:"restroom_id"`
	UserID      string `json:"user_id"`
	Rating      string `json:"rating"`
	Comment     string `json:"comment"`
	PhotoBase64 string `json:"photo_base64"`
}

// ฟังก์ชันเชื่อมต่อกับ Database
func initDatabase() {
	var err error
	dsn := "host=postgres user=postgres password=peempleng123 dbname=ku-toilet port=5432 sslmode=disable"
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// อัพเดทคอลัมน์ base64 เป็น TEXT (ถ้ายังไม่ได้ทำ)
	db.Exec("ALTER TABLE photos ALTER COLUMN base64 TYPE TEXT;")

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
		Email      string `json:"email"`
		GivenName  string `json:"given_name"`
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

// ฟังก์ชันแปลงไฟล์เป็น base64
func ConvertToBase64(fileData io.Reader) (string, error) {
	// อ่านข้อมูลไฟล์ทั้งหมด
	fileBytes, err := io.ReadAll(fileData)
	if err != nil {
		fmt.Println("❌ ERROR: อ่านไฟล์ไม่สำเร็จ:", err)
		return "", fmt.Errorf("อ่านไฟล์ไม่สำเร็จ: %v", err)
	}

	// ตรวจสอบประเภทไฟล์เพื่อกำหนด MIME type ที่ถูกต้อง
	mimeType := http.DetectContentType(fileBytes)

	// แปลงเป็น base64
	base64Data := base64.StdEncoding.EncodeToString(fileBytes)
	
	// เพิ่ม prefix สำหรับแสดงผลรูปภาพในรูปแบบ base64 data URL
	base64URL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
	
	fmt.Println("✅ แปลงไฟล์เป็น base64 สำเร็จ (ขนาด:", len(base64URL), "bytes)")
	return base64URL, nil
}

// แก้ไขฟังก์ชัน CreateReviewWithBase64 ให้ใช้ base64 โดยตรง
func CreateReviewWithBase64(c *fiber.Ctx) error {
	// แสดงข้อมูล request ทั้งหมดเพื่อการ debug
	body := c.Body()
	fmt.Println("Raw request body:", string(body))

	// รับข้อมูล JSON จาก request body
	var requestData ReviewBase64Request
	if err := c.BodyParser(&requestData); err != nil {
		fmt.Println("❌ ERROR: Failed to parse JSON:", err)
		fmt.Println("Request body:", string(body))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request data: " + err.Error()})
	}

	// เพิ่ม debug log เพื่อตรวจสอบข้อมูลที่ได้รับ
	fmt.Println("🔹 Parsed data - RestroomID:", requestData.RestroomID)
	fmt.Println("🔹 Parsed data - UserID:", requestData.UserID)
	fmt.Println("🔹 Parsed data - Rating:", requestData.Rating)
	fmt.Println("🔹 Parsed data - Comment:", requestData.Comment)
	fmt.Println("🔹 Parsed data - Has Photo:", requestData.PhotoBase64 != "")

	// แปลงข้อมูลตัวเลขจาก string เป็นตัวเลข
	restroomID, err := strconv.Atoi(requestData.RestroomID)
	if err != nil {
		fmt.Println("❌ ERROR: Invalid restroom ID:", requestData.RestroomID)
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid restroom ID"})
	}

	userID, err := strconv.Atoi(requestData.UserID)
	if err != nil {
		fmt.Println("❌ ERROR: Invalid user ID:", requestData.UserID)
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	rating, err := strconv.ParseFloat(requestData.Rating, 64)
	if err != nil {
		fmt.Println("❌ ERROR: Invalid rating:", requestData.Rating)
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid rating"})
	}

	// ดึงวันที่ปัจจุบัน (เฉพาะวันที่ ไม่รวมเวลา)
	now := time.Now()
	currentDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// บันทึกข้อมูลรีวิวลงในฐานข้อมูล
	review := Review{
		RestroomID: uint(restroomID),
		UserID:     uint(userID),
		Rating:     rating,
		Comment:    requestData.Comment,
		ReviewDate: currentDate, // เพิ่มวันที่รีวิว
	}

	// สร้างรีวิวก่อน เพื่อให้ได้ review_id
	result := db.Create(&review)
	if result.Error != nil {
		fmt.Println("❌ ERROR: Failed to insert review into database:", result.Error)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save review"})
	}

	fmt.Println("✅ Review successfully saved with ID:", review.ReviewID)

	// ถ้ามีรูปภาพแบบ base64 ให้บันทึกลงฐานข้อมูลโดยตรง
	var photoURL string
	if requestData.PhotoBase64 != "" {
		// ตรวจสอบความยาวของข้อมูล base64
		fmt.Println("🔹 Base64 data length:", len(requestData.PhotoBase64))

		// ตรวจสอบว่ามี data: prefix หรือไม่
		base64Data := requestData.PhotoBase64
		if !strings.Contains(base64Data, "data:") {
			// ถ้าไม่มี data: prefix ให้เพิ่ม
			base64Data = "data:image/jpeg;base64," + base64Data
		}

		// บันทึกข้อมูล base64 ลงฐานข้อมูลโดยตรง
		reviewID := review.ReviewID
		photo := Photo{
			Base64:        base64Data,
			PhotoRestroom: nil,
			PhotoReview:   &reviewID,
		}

		photoResult := db.Create(&photo)
		if photoResult.Error != nil {
			fmt.Println("❌ ERROR: Failed to save photo:", photoResult.Error)
			return c.JSON(fiber.Map{
				"message":     "Review added successfully, but image saving failed",
				"review_id":   review.ReviewID,
				"error_image": photoResult.Error.Error(),
			})
		} else {
			photoURL = base64Data
			fmt.Println("✅ Photo saved successfully with ID:", photo.PhotoID, "linked to review:", reviewID)
		}
	}

	// ดึงข้อมูลห้องน้ำเพื่อส่งกลับไปแสดงผล
	var restroom Restroom
	db.First(&restroom, restroomID)

	// ดึงข้อมูลผู้ใช้
	var user User
	db.First(&user, userID)

	return c.JSON(fiber.Map{
		"message":     "Review added successfully!",
		"review_id":   review.ReviewID,
		"restroom_id": restroomID,
		"user_id":     userID,
		"rating":      rating,
		"comment":     requestData.Comment,
		"name":        restroom.BuildingName,
		"username":    user.FirstName + " " + user.LastName,
		"photo_url":   photoURL,
		"review_date": currentDate.Format("2006-01-02"), // เพิ่มวันที่ในรูปแบบ yyyy-mm-dd
	})
}

// แก้ไขฟังก์ชัน CreateReview ให้ใช้ base64 แทน Google Drive
func CreateReview(c *fiber.Ctx) error {
	// ดึงข้อมูลจาก form
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

	// ดึงวันที่ปัจจุบัน (เฉพาะวันที่ ไม่รวมเวลา)
	now := time.Now()
	currentDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// บันทึกรีวิวลงฐานข้อมูล
	review := Review{
		RestroomID: uint(restroomID),
		UserID:     uint(userID),
		Rating:     rating,
		Comment:    comment,
		ReviewDate: currentDate, // เพิ่มวันที่รีวิว
	}

	result := db.Create(&review)
	if result.Error != nil {
		fmt.Println("❌ ERROR: Failed to insert review into database:", result.Error)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save review"})
	}

	fmt.Println("✅ Review successfully saved! Review ID:", review.ReviewID)

	// ตรวจสอบว่ามีไฟล์รูปภาพหรือไม่
	file, err := c.FormFile("photo")

	var photoURL string

	if err == nil { // ถ้าไม่มี error แสดงว่ามีไฟล์รูปภาพ
		fileData, err := file.Open()
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Cannot open file"})
		}
		defer fileData.Close()

		// แปลงรูปภาพเป็น base64
		base64Data, err := ConvertToBase64(fileData)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to convert to base64"})
		}

		// บันทึกข้อมูลรูปภาพลงฐานข้อมูล
		photo := Photo{
			Base64:        base64Data,
			PhotoRestroom: nil,
			PhotoReview:   &review.ReviewID, // บันทึก review_id ของความคิดเห็นนี้
		}

		photoResult := db.Create(&photo)
		if photoResult.Error != nil {
			fmt.Println("❌ ERROR: Failed to save photo:", photoResult.Error)
		} else {
			photoURL = base64Data
			fmt.Println("✅ Photo saved successfully! Photo ID:", photo.PhotoID)
		}
	}

	// ดึงข้อมูลห้องน้ำเพื่อส่งกลับไปแสดงผล
	var restroom Restroom
	db.First(&restroom, restroomID)

	// ดึงข้อมูลผู้ใช้
	var user User
	db.First(&user, userID)

	return c.JSON(fiber.Map{
		"message":     "Review added successfully!",
		"review_id":   review.ReviewID,
		"restroom_id": restroomID,
		"user_id":     userID,
		"rating":      rating,
		"comment":     comment,
		"name":        restroom.BuildingName,
		"username":    user.FirstName + " " + user.LastName,
		"photo_url":   photoURL,
		"review_date": currentDate.Format("2006-01-02"), // เพิ่มวันที่ในรูปแบบ yyyy-mm-dd
	})
}

func getAllReviewsForAdmin(c *fiber.Ctx) error {
	// ตรวจสอบว่าผู้ใช้มีสิทธิ์แอดมินหรือไม่
	email := c.Get("X-User-Email", "")
	if email == "" {
		// ถ้าไม่มี header ลองดึงจาก query
		email = c.Query("email", "")
	}

	// ตรวจสอบว่าเป็นอีเมลแอดมินหรือไม่
	if email != "admkutoilet@gmail.com" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "คุณไม่มีสิทธิ์เข้าถึงข้อมูลส่วนนี้",
		})
	}

	// คำสั่ง SQL เพื่อดึงข้อมูลรีวิวทั้งหมดพร้อมข้อมูลผู้ใช้และห้องน้ำ
	rows, err := db.Raw(`
        SELECT r.review_id, r.restroom_id, r.user_id, r.rating, r.comment, r.review_date, 
               u.first_name, u.last_name, u.email,
               rs.building_name, rs.floor
        FROM reviews r
        JOIN users u ON r.user_id = u.user_id
        JOIN restrooms rs ON r.restroom_id = rs.restroom_id
        ORDER BY r.review_date DESC
    `).Rows()

	if err != nil {
		fmt.Println("❌ เกิดข้อผิดพลาดในการดึงข้อมูลรีวิว:", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "เกิดข้อผิดพลาดในการดึงข้อมูลรีวิว",
		})
	}
	defer rows.Close()

	// สร้าง slice เพื่อเก็บข้อมูลรีวิว
	var reviews []fiber.Map

	// วนลูปอ่านข้อมูลทีละแถว
	for rows.Next() {
		var reviewID, restroomID, userID uint
		var rating float64
		var comment, firstName, lastName, email, buildingName string
		var floor int
		var reviewDate time.Time

		if err := rows.Scan(&reviewID, &restroomID, &userID, &rating, &comment, &reviewDate,
			&firstName, &lastName, &email, &buildingName, &floor); err != nil {
			fmt.Println("❌ เกิดข้อผิดพลาดในการอ่านข้อมูลรีวิว:", err)
			continue
		}

		// ดึงรูปภาพของรีวิว (ถ้ามี)
		var photos []Photo
		db.Where("photo_review = ?", reviewID).Find(&photos)

		// เพิ่มข้อมูลรีวิวลงใน slice
		review := fiber.Map{
			"review_id":     reviewID,
			"restroom_id":   restroomID,
			"user_id":       userID,
			"rating":        rating,
			"comment":       comment,
			"review_date":   reviewDate.Format("2006-01-02"), // เพิ่มวันที่รีวิวในรูปแบบ yyyy-mm-dd
			"first_name":    firstName,
			"last_name":     lastName,
			"email":         email,
			"building_name": buildingName,
			"floor":         floor,
		}

		// เพิ่มข้อมูลรูปภาพถ้ามี
		if len(photos) > 0 {
			review["photo_url"] = photos[0].Base64 // ใช้รูปแรก
		}

		reviews = append(reviews, review)
	}

	return c.JSON(reviews)
}

func deleteReviewForAdmin(c *fiber.Ctx) error {
	// ตรวจสอบว่าผู้ใช้มีสิทธิ์แอดมินหรือไม่
	email := c.Get("X-User-Email", "")
	if email == "" {
		// ถ้าไม่มี header ลองดึงจาก query หรือ body
		email = c.Query("email", "")
	}

	// ตรวจสอบว่าเป็นอีเมลแอดมินหรือไม่
	if email != "admkutoilet@gmail.com" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "คุณไม่มีสิทธิ์ลบข้อมูลรีวิว",
		})
	}

	// ดึง ID ของรีวิวที่ต้องการลบ
	reviewID := c.Params("id")
	if reviewID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "ไม่พบรหัสรีวิวที่ต้องการลบ",
		})
	}

	// ตรวจสอบว่ามีรีวิวนี้อยู่จริงหรือไม่
	var review Review
	if err := db.First(&review, reviewID).Error; err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "ไม่พบรีวิวที่ต้องการลบ",
		})
	}

	// ลบรูปภาพที่เกี่ยวข้องกับรีวิวนี้ก่อน
	if err := db.Where("photo_review = ?", reviewID).Delete(&Photo{}).Error; err != nil {
		fmt.Println("❌ เกิดข้อผิดพลาดในการลบรูปภาพของรีวิว:", err)
		// ไม่ return error เพื่อให้ยังสามารถลบรีวิวได้
	}

	// ลบรีวิว
	if err := db.Delete(&review).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "เกิดข้อผิดพลาดในการลบรีวิว",
		})
	}

	return c.JSON(fiber.Map{
		"message":   "ลบรีวิวสำเร็จ",
		"review_id": review.ReviewID,
	})
}

// เพิ่มฟังก์ชันย้ายข้อมูลรูปภาพจาก Google Drive เป็น base64
func migratePhotosToBase64(c *fiber.Ctx) error {
	// ตรวจสอบว่าผู้ใช้มีสิทธิ์แอดมินหรือไม่
	email := c.Query("email", "")
	if email != "admkutoilet@gmail.com" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "คุณไม่มีสิทธิ์เข้าถึงฟังก์ชันนี้",
		})
	}

	// ตรวจสอบจำนวนรูปที่ต้องการย้าย
	var count int64
	db.Model(&Photo{}).
		Where("base64 LIKE 'https://drive.google.com%' OR base64 LIKE 'https://lh3.googleusercontent.com%'").
		Count(&count)

	// ถ้าไม่มีรูปที่ต้องย้าย
	if count == 0 {
		return c.JSON(fiber.Map{
			"message": "ไม่พบรูปภาพที่ต้องการย้าย",
			"count":   0,
		})
	}

	// แสดงจำนวนรูปที่จะย้าย
	fmt.Printf("🔹 พบรูปภาพที่ต้องการย้ายทั้งหมด %d รูป\n", count)

	// ส่งคืนข้อมูลเบื้องต้น
	return c.JSON(fiber.Map{
		"message": "พบรูปภาพที่ต้องย้าย",
		"count":   count,
		"note":    "กระบวนการย้ายข้อมูลจะใช้เวลานาน กรุณาใช้ endpoint /admin/migratePhotos/start เพื่อเริ่มการย้ายข้อมูล",
	})
}

// ฟังก์ชันเริ่มการย้ายข้อมูล
func startMigratePhotosToBase64(c *fiber.Ctx) error {
	// ตรวจสอบว่าผู้ใช้มีสิทธิ์แอดมินหรือไม่
	email := c.Query("email", "")
	if email != "admkutoilet@gmail.com" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "คุณไม่มีสิทธิ์เข้าถึงฟังก์ชันนี้",
		})
	}

	// สร้าง HTTP client สำหรับดาวน์โหลดรูปภาพ
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// จำกัดจำนวนรูปภาพที่จะประมวลผลต่อครั้ง (batch)
	limit := 10
	limitStr := c.Query("limit", "10")
	limit, _ = strconv.Atoi(limitStr)

	// ดึงรูปภาพที่เป็น URL Google Drive
	var photos []Photo
	result := db.Where("base64 LIKE 'https://drive.google.com%' OR base64 LIKE 'https://lh3.googleusercontent.com%'").
		Limit(limit).
		Find(&photos)

	if result.Error != nil {
		fmt.Println("❌ เกิดข้อผิดพลาดในการดึงข้อมูลรูปภาพ:", result.Error)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "เกิดข้อผิดพลาดในการดึงข้อมูลรูปภาพ",
		})
	}

	if len(photos) == 0 {
		return c.JSON(fiber.Map{
			"message": "ไม่พบรูปภาพที่ต้องการย้าย",
			"count":   0,
		})
	}

	fmt.Printf("🔹 กำลังย้ายข้อมูลรูปภาพจำนวน %d รูป\n", len(photos))

	// ข้อมูลสถิติ
	successCount := 0
	errorCount := 0
	var errors []string

	// วนลูปประมวลผลรูปภาพ
	for i, photo := range photos {
		fmt.Printf("🔹 กำลังประมวลผลรูปที่ %d/%d (ID: %d)\n", i+1, len(photos), photo.PhotoID)

		// ดาวน์โหลดรูปจาก URL
		resp, err := client.Get(photo.Base64)
		if err != nil {
			fmt.Printf("❌ ไม่สามารถดาวน์โหลดรูปได้: %v\n", err)
			errorCount++
			errors = append(errors, fmt.Sprintf("ID %d: %v", photo.PhotoID, err))
			continue
		}

		// ตรวจสอบสถานะการดาวน์โหลด
		if resp.StatusCode != http.StatusOK {
			errMsg := fmt.Sprintf("ดาวน์โหลดรูปไม่สำเร็จ: HTTP %d", resp.StatusCode)
			fmt.Println("❌", errMsg)
			resp.Body.Close()
			errorCount++
			errors = append(errors, fmt.Sprintf("ID %d: %s", photo.PhotoID, errMsg))
			continue
		}

		// อ่านข้อมูลรูปภาพ
		imgData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("❌ ไม่สามารถอ่านข้อมูลรูปได้: %v\n", err)
			errorCount++
			errors = append(errors, fmt.Sprintf("ID %d: %v", photo.PhotoID, err))
			continue
		}

		// ตรวจสอบขนาดรูปและประเภทไฟล์
		fileSize := len(imgData)
		contentType := http.DetectContentType(imgData)
		fmt.Printf("🔹 ขนาดรูป: %d bytes, ประเภท: %s\n", fileSize, contentType)

		// ถ้ารูปขนาดเล็กเกินไปหรือไม่ใช่รูปภาพ ให้ข้าม
		if fileSize < 100 || !strings.HasPrefix(contentType, "image/") {
			fmt.Println("⚠️ รูปภาพไม่ถูกต้องหรือมีขนาดเล็กเกินไป")
			errorCount++
			errors = append(errors, fmt.Sprintf("ID %d: รูปภาพไม่ถูกต้องหรือมีขนาดเล็กเกินไป", photo.PhotoID))
			continue
		}

		// แปลงเป็น base64
		base64Data := base64.StdEncoding.EncodeToString(imgData)
		base64URL := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

		// อัพเดทข้อมูลลงฐานข้อมูล
		updateResult := db.Model(&Photo{}).Where("photo_id = ?", photo.PhotoID).Update("base64", base64URL)
		if updateResult.Error != nil {
			fmt.Printf("❌ ไม่สามารถอัพเดทข้อมูลได้: %v\n", updateResult.Error)
			errorCount++
			errors = append(errors, fmt.Sprintf("ID %d: %v", photo.PhotoID, updateResult.Error))
			continue
		}

		fmt.Printf("✅ อัพเดทรูปสำเร็จ: ID %d\n", photo.PhotoID)
		successCount++

		// หยุดพักเล็กน้อยเพื่อไม่ให้ทำงานหนักเกินไป
		time.Sleep(500 * time.Millisecond)
	}

	// สรุปผลการทำงาน
	return c.JSON(fiber.Map{
		"message":        "การย้ายข้อมูลเสร็จสมบูรณ์",
		"total":          len(photos),
		"success_count":  successCount,
		"error_count":    errorCount,
		"errors":         errors,
		"remaining_info": "ใช้ endpoint เดิมซ้ำเพื่อย้ายข้อมูลชุดต่อไป",
	})
}

// ปรับปรุงการตั้งค่าเซิร์ฟเวอร์ในฟังก์ชัน main
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)

	// เชื่อมต่อฐานข้อมูล
	initDatabase()

	app := fiber.New(fiber.Config{
		// เพิ่มขนาด body size เพื่อรองรับการส่งรูปภาพขนาดใหญ่
		BodyLimit: 10 * 1024 * 1024, // 10MB
		// เพิ่มเวลา timeout สำหรับการอัปโหลดขนาดใหญ่
		ReadTimeout: 60 * time.Second,
	})

	// ตั้งค่า CORS ให้ถูกต้อง - เปิดการเข้าถึงจากหลาย origin
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*", 
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-User-Email",
		ExposeHeaders:    "Content-Length, Content-Type",
		AllowCredentials: false,
		MaxAge:           86400,
	}))

	// เพิ่ม middleware เพื่อแสดง request path และ method (เพื่อการ debug)
	app.Use(func(c *fiber.Ctx) error {
		fmt.Println("🔷 Request:", c.Method(), c.Path())
		return c.Next()
	})

	// แสดงข้อความเริ่มต้นเซิร์ฟเวอร์
	log.Println("✅ Server starting...")
	fmt.Println("✅ Server starting...")

	// ลงทะเบียน routes
	app.Post("/auth/google", googleAuthHandler)

	// API สำหรับดึงข้อมูลห้องน้ำ
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
						"review_id":   review.ReviewID,
						"restroom_id": review.RestroomID,
						"user_id":     review.UserID,
						"first_name":  firstName, // ✅ เพิ่มชื่อที่นี่
						"last_name":   lastName,  // ✅ เพิ่มนามสกุลที่นี่
						"rating":      review.Rating,
						"comment":     review.Comment,
						"review_date": review.ReviewDate.Format("2006-01-02"), // เพิ่มวันที่รีวิวในรูปแบบ yyyy-mm-dd
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

	// ลงทะเบียน route สำหรับการเพิ่มรีวิว
	fmt.Println("🔶 กำลังลงทะเบียน route POST /review/base64")
	app.Post("/review/base64", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ POST /review/base64")
		return CreateReviewWithBase64(c)
	})

	fmt.Println("🔶 กำลังลงทะเบียน route POST /review")
	app.Post("/review", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ POST /review")
		return CreateReview(c)
	})

	// เพิ่ม route สำหรับตรวจสอบว่าเซิร์ฟเวอร์ทำงานอยู่
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "ok",
			"message":   "Server is running",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// เพิ่ม route สำหรับทดสอบการส่งข้อมูล base64
	app.Post("/test-base64", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ POST /test-base64")
		body := string(c.Body())

		// ปรับปรุงการแสดงผล body เพื่อป้องกัน panic
		preview := body
		if len(body) > 100 {
			preview = body[:100] + "..."
		}
		fmt.Println("Body (ตัดออกบางส่วน):", preview)

		return c.JSON(fiber.Map{
			"status":         "ok",
			"message":        "Test base64 endpoint working",
			"received_bytes": len(body),
		})
	})

	fmt.Println("🔶 กำลังลงทะเบียน route สำหรับแอดมิน")

	// API สำหรับดึงรายการรีวิวทั้งหมด (สำหรับแอดมิน)
	app.Get("/admin/reviews", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ GET /admin/reviews")
		return getAllReviewsForAdmin(c)
	})

	// API สำหรับลบรีวิว (สำหรับแอดมิน)
	app.Delete("/admin/reviews/:id", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ DELETE /admin/reviews/:id")
		return deleteReviewForAdmin(c)
	})

	// API สำหรับตรวจสอบรูปภาพที่ต้องย้าย
	app.Get("/admin/migratePhotos", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ GET /admin/migratePhotos")
		return migratePhotosToBase64(c)
	})

	// API สำหรับเริ่มการย้ายข้อมูลรูปภาพ
	app.Get("/admin/migratePhotos/start", func(c *fiber.Ctx) error {
		fmt.Println("🟢 ได้รับ request สำหรับ GET /admin/migratePhotos/start")
		return startMigratePhotosToBase64(c)
	})

	// เริ่มต้นเซิร์ฟเวอร์บนพอร์ต 3001
	log.Println("✅ Server listening on port 3001")
	fmt.Println("🚀 กำลังเริ่มต้นเซิร์ฟเวอร์ที่พอร์ต 3001...")
	log.Fatal(app.Listen(":3001"))
}