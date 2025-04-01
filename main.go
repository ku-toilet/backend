package main

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"

	"context"
	"encoding/base64"
	"io"
	"strings"
	"time"

	"strconv"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
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

// ตาราง Photo
type Photo struct {
	PhotoID       uint   `json:"photo_id" gorm:"primaryKey;autoIncrement"`
	Base64        string `json:"base64" gorm:"not null"`
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

	// ถ้ามีรูปภาพแบบ base64 ให้แปลงและบันทึก
	var photoURL string
	if requestData.PhotoBase64 != "" {
		// ตรวจสอบความยาวของข้อมูล base64
		fmt.Println("🔹 Base64 data length:", len(requestData.PhotoBase64))

		// แยกข้อมูล base64 ออกจาก header (ถ้ามี)
		base64Data := requestData.PhotoBase64
		if strings.Contains(base64Data, ";base64,") {
			parts := strings.Split(base64Data, ";base64,")
			if len(parts) == 2 {
				base64Data = parts[1]
				fmt.Println("🔹 Base64 prefix detected and stripped")
			}
		}

		// แปลงข้อมูล base64 เป็น binary
		imgData, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			fmt.Println("❌ ERROR: Failed to decode base64 image:", err)
			// แม้มีปัญหากับรูปภาพ แต่เรายังคงสร้างรีวิวไปแล้ว จึงส่งคืนข้อมูลรีวิวโดยไม่มีรูป
			return c.JSON(fiber.Map{
				"message":     "Review added successfully, but image processing failed",
				"review_id":   review.ReviewID,
				"error_image": err.Error(),
			})
		}

		// สร้างชื่อไฟล์ชั่วคราว
		tmpFileName := fmt.Sprintf("review_%d_%d.jpg", review.ReviewID, time.Now().Unix())

		// สร้างไฟล์ชั่วคราวเพื่อใช้ในการอัปโหลด
		tempFile, err := os.CreateTemp("", tmpFileName)
		if err != nil {
			fmt.Println("❌ ERROR: Failed to create temp file:", err)
			// ส่งคืนข้อมูลรีวิวโดยไม่มีรูป
			return c.JSON(fiber.Map{
				"message":     "Review added successfully, but image processing failed",
				"review_id":   review.ReviewID,
				"error_image": "Failed to create temporary file",
			})
		}
		defer os.Remove(tempFile.Name())

		// เขียนข้อมูลรูปภาพลงไฟล์ชั่วคราว
		if _, err := tempFile.Write(imgData); err != nil {
			fmt.Println("❌ ERROR: Failed to write to temp file:", err)
			tempFile.Close()
			// ส่งคืนข้อมูลรีวิวโดยไม่มีรูป
			return c.JSON(fiber.Map{
				"message":     "Review added successfully, but image processing failed",
				"review_id":   review.ReviewID,
				"error_image": "Failed to write image data",
			})
		}

		// ปิดไฟล์เพื่อให้แน่ใจว่าข้อมูลถูกเขียนลงดิสก์
		tempFile.Close()

		// เปิดไฟล์เพื่ออ่านข้อมูล
		fileData, err := os.Open(tempFile.Name())
		if err != nil {
			fmt.Println("❌ ERROR: Failed to open temp file:", err)
			// ส่งคืนข้อมูลรีวิวโดยไม่มีรูป
			return c.JSON(fiber.Map{
				"message":     "Review added successfully, but image processing failed",
				"review_id":   review.ReviewID,
				"error_image": "Failed to read image data",
			})
		}
		defer fileData.Close()

		// อัปโหลดรูปไป Google Drive
		driveLink, err := UploadFileToDrive(tmpFileName, fileData, "1P4Jks1kHKduS3yg7mk2uBXqd6EGEmPtI")
		if err != nil {
			fmt.Println("❌ ERROR: Failed to upload to Google Drive:", err)
			// ส่งคืนข้อมูลรีวิวโดยไม่มีรูป
			return c.JSON(fiber.Map{
				"message":     "Review added successfully, but image upload failed",
				"review_id":   review.ReviewID,
				"error_image": "Failed to upload to Google Drive",
			})
		}

		// บันทึกข้อมูลรูปภาพลงฐานข้อมูล - ดูให้แน่ใจว่า photo_review ถูกตั้งเป็น review.ReviewID
		reviewID := review.ReviewID // สำคัญ: ใช้ ID จากรีวิวที่สร้างไปแล้ว
		photo := Photo{
			Base64:        driveLink,
			PhotoRestroom: nil,
			PhotoReview:   &reviewID, // เชื่อมโยงกับ review_id
		}

		photoResult := db.Create(&photo)
		if photoResult.Error != nil {
			fmt.Println("❌ ERROR: Failed to save photo:", photoResult.Error)
			// ส่งคืนข้อมูลรีวิวโดยไม่มีรูป
			return c.JSON(fiber.Map{
				"message":   "Review added successfully, but failed to save photo in database",
				"review_id": review.ReviewID,
				"error_db":  photoResult.Error.Error(),
			})
		} else {
			photoURL = driveLink
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

// ฟังก์ชันอัปโหลดรูปไป Google Drive
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

		// อัปโหลดรูปไป Google Drive
		driveLink, err := UploadFileToDrive(file.Filename, fileData, "1P4Jks1kHKduS3yg7mk2uBXqd6EGEmPtI")
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Google Drive upload failed"})
		}

		// บันทึกข้อมูลรูปภาพลงฐานข้อมูล
		photo := Photo{
			Base64:        driveLink,
			PhotoRestroom: nil,
			PhotoReview:   &review.ReviewID, // บันทึก review_id ของความคิดเห็นนี้
		}

		photoResult := db.Create(&photo)
		if photoResult.Error != nil {
			fmt.Println("❌ ERROR: Failed to save photo:", photoResult.Error)
		} else {
			photoURL = driveLink
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
			photoURLs := make([]string, len(photos))
			for i, photo := range photos {
				photoURLs[i] = photo.Base64
			}
			review["photo_url"] = photoURLs[0] // ใช้รูปแรก
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

	// เปลี่ยนจากลิงค์ดู (view) เป็นลิงค์รูปขนาดย่อ (thumbnail)
	link := "https://drive.google.com/thumbnail?id=" + file.Id + "&sz=w1000"

	fmt.Println("✅ SUCCESS: File uploaded:", link)
	return link, nil
}

// ปรับปรุงการตั้งค่าเซิร์ฟเวอร์ในฟังก์ชัน main
// แก้ไขส่วนการลงทะเบียน route ในฟังก์ชัน main
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

	// เริ่มต้นเซิร์ฟเวอร์บนพอร์ต 3001
	log.Println("✅ Server listening on port 3001")
	fmt.Println("🚀 กำลังเริ่มต้นเซิร์ฟเวอร์ที่พอร์ต 3001...")
	log.Fatal(app.Listen(":3001"))
}