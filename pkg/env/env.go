package env

import (
	"log"

	"github.com/joho/godotenv"
)

func init() {
	// 加载 .env 文件
	// 这个 init() 会在其他依赖包之前执行
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}
}
