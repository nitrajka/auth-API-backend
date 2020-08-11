package api

import (
	"github.com/gin-gonic/gin"
	"github.com/nitrajka/al_ny/pkg/auth"
	"github.com/nitrajka/al_ny/pkg/db"
)

type app struct {
	db.Database
	auth auth.Authentication
	oauthGoogleUrlAPI string
}

type Application interface {
	Login(c *gin.Context)
	Logout(c *gin.Context)
	Signup(c *gin.Context)
	GoogleLogin(c *gin.Context)
	UpdateUser(c *gin.Context)
	GetUserById(c *gin.Context)
	TokenAuthMiddleWare() gin.HandlerFunc
}

func NewApp(datab db.Database, aut auth.Authentication) (Application, error) {
	return &app{
		datab,
		aut,
		"https://www.googleapis.com/oauth2/v2/userinfo?access_token=",
	}, nil
}