# Go + Fiber Roadmap

> Lộ trình học Go và Fiber framework cho backend development

---

## Phase 1: Go Fundamentals (1-2 tuần)

### 1.1 Setup & Basics
- [ ] Cài đặt Go (https://go.dev/dl/)
- [ ] Setup VS Code + Go extension
- [ ] Hiểu `GOPATH`, `GOROOT`, Go modules

```bash
# Kiểm tra cài đặt
go version

# Tạo project mới
mkdir my-project && cd my-project
go mod init my-project
```

### 1.2 Syntax cơ bản
```go
package main

import "fmt"

func main() {
    // Variables
    var name string = "40Study"
    age := 25  // Short declaration

    // Constants
    const PI = 3.14

    // Print
    fmt.Println("Hello", name)
    fmt.Printf("Age: %d\n", age)
}
```

**Học:**
- [ ] Variables & Constants
- [ ] Data types: `string`, `int`, `float64`, `bool`
- [ ] Arrays & Slices
- [ ] Maps
- [ ] Conditionals (`if`, `switch`)
- [ ] Loops (`for` - Go chỉ có `for`)

### 1.3 Functions
```go
// Basic function
func add(a, b int) int {
    return a + b
}

// Multiple returns
func divide(a, b float64) (float64, error) {
    if b == 0 {
        return 0, errors.New("cannot divide by zero")
    }
    return a / b, nil
}

// Variadic function
func sum(nums ...int) int {
    total := 0
    for _, n := range nums {
        total += n
    }
    return total
}
```

### 1.4 Structs & Methods
```go
// Struct
type User struct {
    ID       int
    Username string
    Email    string
    IsActive bool
}

// Method
func (u *User) FullInfo() string {
    return fmt.Sprintf("%s (%s)", u.Username, u.Email)
}

// Constructor pattern
func NewUser(username, email string) *User {
    return &User{
        Username: username,
        Email:    email,
        IsActive: true,
    }
}
```

### 1.5 Interfaces
```go
// Interface definition
type Repository interface {
    Create(item interface{}) error
    FindByID(id int) (interface{}, error)
    Update(id int, item interface{}) error
    Delete(id int) error
}

// Implement interface
type UserRepository struct {
    db *sql.DB
}

func (r *UserRepository) Create(item interface{}) error {
    // Implementation
    return nil
}
```

### 1.6 Error Handling
```go
// Go không có try-catch, dùng multiple returns
result, err := someFunction()
if err != nil {
    // Handle error
    log.Printf("Error: %v", err)
    return err
}

// Custom errors
type NotFoundError struct {
    Resource string
    ID       int
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s with ID %d not found", e.Resource, e.ID)
}
```

### 1.7 Pointers
```go
func main() {
    x := 10
    p := &x  // p là pointer tới x

    fmt.Println(*p)  // 10 - dereference
    *p = 20
    fmt.Println(x)   // 20 - x đã thay đổi
}

// Khi nào dùng pointer?
// - Muốn modify value gốc
// - Struct lớn (tránh copy)
// - Method cần modify receiver
```

---

## Phase 2: Go Advanced (1-2 tuần)

### 2.1 Goroutines & Channels
```go
// Goroutine - lightweight thread
go func() {
    fmt.Println("Running in goroutine")
}()

// Channel - communication between goroutines
ch := make(chan string)

go func() {
    ch <- "Hello from goroutine"
}()

msg := <-ch
fmt.Println(msg)

// Buffered channel
ch := make(chan int, 3)  // Buffer size 3

// Select - listen multiple channels
select {
case msg := <-ch1:
    fmt.Println("From ch1:", msg)
case msg := <-ch2:
    fmt.Println("From ch2:", msg)
case <-time.After(time.Second):
    fmt.Println("Timeout")
}
```

### 2.2 Context
```go
import "context"

// Context với timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Truyền context vào function
func fetchData(ctx context.Context) error {
    select {
    case <-ctx.Done():
        return ctx.Err()  // Timeout hoặc cancelled
    default:
        // Do work
        return nil
    }
}
```

### 2.3 Packages & Modules
```
myproject/
├── go.mod
├── go.sum
├── main.go
├── internal/           # Private packages
│   └── database/
│       └── db.go
└── pkg/               # Public packages
    └── utils/
        └── helpers.go
```

```go
// go.mod
module github.com/username/myproject

go 1.21

require (
    github.com/gofiber/fiber/v2 v2.52.0
)
```

### 2.4 Testing
```go
// user_test.go
package user

import "testing"

func TestAdd(t *testing.T) {
    result := Add(2, 3)
    if result != 5 {
        t.Errorf("Expected 5, got %d", result)
    }
}

// Table-driven tests
func TestAddTable(t *testing.T) {
    tests := []struct {
        a, b, expected int
    }{
        {1, 2, 3},
        {0, 0, 0},
        {-1, 1, 0},
    }

    for _, tt := range tests {
        result := Add(tt.a, tt.b)
        if result != tt.expected {
            t.Errorf("Add(%d, %d) = %d; want %d", tt.a, tt.b, result, tt.expected)
        }
    }
}
```

```bash
# Run tests
go test ./...
go test -v ./...           # Verbose
go test -cover ./...       # Coverage
```

---

## Phase 3: Fiber Framework (1-2 tuần)

### 3.1 Fiber Basics
```bash
go get github.com/gofiber/fiber/v2
```

```go
package main

import (
    "github.com/gofiber/fiber/v2"
)

func main() {
    app := fiber.New()

    // Basic route
    app.Get("/", func(c *fiber.Ctx) error {
        return c.SendString("Hello, World!")
    })

    // JSON response
    app.Get("/api/health", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{
            "status": "ok",
            "message": "Server is running",
        })
    })

    app.Listen(":3000")
}
```

### 3.2 Routing
```go
// Route parameters
app.Get("/users/:id", func(c *fiber.Ctx) error {
    id := c.Params("id")
    return c.JSON(fiber.Map{"user_id": id})
})

// Query parameters
// GET /search?q=golang&page=1
app.Get("/search", func(c *fiber.Ctx) error {
    query := c.Query("q")
    page := c.QueryInt("page", 1)  // Default 1
    return c.JSON(fiber.Map{"query": query, "page": page})
})

// Route groups
api := app.Group("/api")
v1 := api.Group("/v1")

v1.Get("/users", getUsers)
v1.Post("/users", createUser)
v1.Get("/users/:id", getUser)
v1.Put("/users/:id", updateUser)
v1.Delete("/users/:id", deleteUser)
```

### 3.3 Request Body
```go
// Struct for body parsing
type CreateUserRequest struct {
    Username string `json:"username" validate:"required,min=3"`
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
}

app.Post("/users", func(c *fiber.Ctx) error {
    req := new(CreateUserRequest)

    // Parse JSON body
    if err := c.BodyParser(req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "Invalid request body",
        })
    }

    // Process...
    return c.Status(fiber.StatusCreated).JSON(fiber.Map{
        "message": "User created",
        "user": req,
    })
})
```

### 3.4 Middleware
```go
import (
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
    app := fiber.New()

    // Built-in middleware
    app.Use(logger.New())   // Request logging
    app.Use(recover.New())  // Panic recovery
    app.Use(cors.New())     // CORS

    // Custom middleware
    app.Use(func(c *fiber.Ctx) error {
        fmt.Println("Before request")
        err := c.Next()  // Continue to next handler
        fmt.Println("After request")
        return err
    })
}

// Auth middleware
func AuthMiddleware(c *fiber.Ctx) error {
    token := c.Get("Authorization")
    if token == "" {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "error": "Missing authorization header",
        })
    }

    // Validate token...

    return c.Next()
}

// Apply to routes
app.Get("/protected", AuthMiddleware, protectedHandler)
```

### 3.5 Error Handling
```go
// Custom error handler
app := fiber.New(fiber.Config{
    ErrorHandler: func(c *fiber.Ctx, err error) error {
        code := fiber.StatusInternalServerError

        if e, ok := err.(*fiber.Error); ok {
            code = e.Code
        }

        return c.Status(code).JSON(fiber.Map{
            "error": err.Error(),
        })
    },
})

// Return errors
app.Get("/users/:id", func(c *fiber.Ctx) error {
    id := c.Params("id")
    user, err := userService.FindByID(id)

    if err != nil {
        return fiber.NewError(fiber.StatusNotFound, "User not found")
    }

    return c.JSON(user)
})
```

---

## Phase 4: Database với GORM (1 tuần)

### 4.1 Setup GORM + PostgreSQL
```bash
go get -u gorm.io/gorm
go get -u gorm.io/driver/postgres
```

```go
import (
    "gorm.io/gorm"
    "gorm.io/driver/postgres"
)

func ConnectDB() (*gorm.DB, error) {
    dsn := "host=localhost user=postgres password=secret dbname=40study port=5432 sslmode=disable"

    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        return nil, err
    }

    return db, nil
}
```

### 4.2 Models
```go
import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type User struct {
    ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    Email        string         `gorm:"uniqueIndex;not null"`
    PasswordHash string         `gorm:"not null"`
    UserName     string         `gorm:"not null"`
    FullName     string
    Role         string         `gorm:"default:student"`
    IsVerified   bool           `gorm:"default:false"`
    IsActive     bool           `gorm:"default:true"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    gorm.DeletedAt `gorm:"index"`  // Soft delete
}

type Course struct {
    ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    InstructorID uuid.UUID `gorm:"type:uuid;not null"`
    Instructor   User      `gorm:"foreignKey:InstructorID"`
    Title        string    `gorm:"not null"`
    Slug         string    `gorm:"uniqueIndex;not null"`
    Price        float64   `gorm:"default:0"`
    Status       string    `gorm:"default:draft"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// Auto migrate
db.AutoMigrate(&User{}, &Course{})
```

### 4.3 CRUD Operations
```go
// Create
user := User{
    Email:    "test@example.com",
    UserName: "testuser",
}
result := db.Create(&user)

// Read
var user User
db.First(&user, "id = ?", id)  // By ID
db.Where("email = ?", email).First(&user)

// Read all with pagination
var users []User
db.Offset(0).Limit(10).Find(&users)

// Update
db.Model(&user).Updates(User{FullName: "New Name"})

// Delete (soft delete)
db.Delete(&user)

// Preload relationships
var course Course
db.Preload("Instructor").First(&course, id)
```

### 4.4 Transactions
```go
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&user).Error; err != nil {
        return err  // Rollback
    }

    if err := tx.Create(&profile).Error; err != nil {
        return err  // Rollback
    }

    return nil  // Commit
})
```

---

## Phase 5: Project Structure (Best Practices)

### 5.1 Clean Architecture
```
backend/
├── cmd/
│   └── api/
│       └── main.go              # Entry point
├── internal/
│   ├── config/
│   │   └── config.go            # App configuration
│   ├── database/
│   │   └── postgres.go          # DB connection
│   ├── models/
│   │   ├── user.go
│   │   └── course.go
│   ├── repositories/
│   │   ├── user_repository.go
│   │   └── course_repository.go
│   ├── services/
│   │   ├── user_service.go
│   │   └── course_service.go
│   ├── handlers/
│   │   ├── user_handler.go
│   │   └── course_handler.go
│   ├── middlewares/
│   │   ├── auth.go
│   │   └── logger.go
│   ├── routes/
│   │   └── routes.go
│   └── utils/
│       ├── response.go
│       └── validator.go
├── pkg/                         # Shared packages
├── docs/
├── go.mod
├── go.sum
└── .env
```

### 5.2 Dependency Injection
```go
// Repository
type UserRepository struct {
    db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
    return &UserRepository{db: db}
}

// Service
type UserService struct {
    repo *UserRepository
}

func NewUserService(repo *UserRepository) *UserService {
    return &UserService{repo: repo}
}

// Handler
type UserHandler struct {
    service *UserService
}

func NewUserHandler(service *UserService) *UserHandler {
    return &UserHandler{service: service}
}

// main.go - Wire up
func main() {
    db := database.Connect()

    userRepo := repositories.NewUserRepository(db)
    userService := services.NewUserService(userRepo)
    userHandler := handlers.NewUserHandler(userService)

    app := fiber.New()
    routes.SetupUserRoutes(app, userHandler)

    app.Listen(":3000")
}
```

---

## Phase 6: Authentication & Security (1 tuần)

### 6.1 JWT Authentication
```bash
go get github.com/golang-jwt/jwt/v5
```

```go
import "github.com/golang-jwt/jwt/v5"

type Claims struct {
    UserID uuid.UUID `json:"user_id"`
    Email  string    `json:"email"`
    Role   string    `json:"role"`
    jwt.RegisteredClaims
}

// Generate token
func GenerateToken(user *User) (string, error) {
    claims := &Claims{
        UserID: user.ID,
        Email:  user.Email,
        Role:   user.Role,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

// Validate token
func ValidateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
        return []byte(os.Getenv("JWT_SECRET")), nil
    })

    if err != nil {
        return nil, err
    }

    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }

    return nil, errors.New("invalid token")
}
```

### 6.2 Password Hashing
```go
import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

---

## Phase 7: Thực hành với 40Study

### Mini Projects để luyện tập:
1. **User Authentication API** - Register, Login, Refresh Token
2. **Course CRUD API** - Full CRUD với pagination
3. **Enrollment System** - Enroll, Track Progress
4. **Quiz System** - Questions, Answers, Attempts

### Checklist hoàn thành:
- [ ] Phase 1: Go Fundamentals
- [ ] Phase 2: Go Advanced
- [ ] Phase 3: Fiber Framework
- [ ] Phase 4: Database với GORM
- [ ] Phase 5: Project Structure
- [ ] Phase 6: Authentication
- [ ] Phase 7: Build 40Study Backend

---

## Resources

### Documentation
- [Go Official Docs](https://go.dev/doc/)
- [Fiber Docs](https://docs.gofiber.io/)
- [GORM Docs](https://gorm.io/docs/)

### Courses & Tutorials
- [Go by Example](https://gobyexample.com/)
- [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests/)
- [Fiber Recipes](https://github.com/gofiber/recipes)

### Books
- "The Go Programming Language" - Donovan & Kernighan
- "Learning Go" - Jon Bodner

---

*Estimated total time: 6-8 tuần (học 2-3h/ngày)*
