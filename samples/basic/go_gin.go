package samples

import (
	"github.com/gin-gonic/gin"
)

func getUsers(c *gin.Context) {
	// Handler logic for GET /users
}

func login(c *gin.Context) {
	// Handler logic for POST /login
}

func main() {

	router := gin.Default()

	router.GET("/users", getUsers)
	router.POST("/login", login)
}
