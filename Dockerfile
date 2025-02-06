#ใช้ Base Image ที่รองรับ Go 1.23
FROM golang:1.23

#ตั้งโฟลเดอร์ทำงานใน container
WORKDIR /app

#คัดลอก go.mod และ go.sum ไปยัง container
COPY go.mod go.sum ./

#ติดตั้ง dependencies
RUN go mod download

#คัดลอกไฟล์โค้ดทั้งหมด
COPY . .

#คอมไพล์ Go เป็นไฟล์ binary
RUN go build -o main .

#เปิดพอร์ต 3001
EXPOSE 3001

#คำสั่งเริ่มต้นเมื่อ container รัน
CMD ["./main"]