package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/tabrizihamid84/ecommerce/database"
	"github.com/tabrizihamid84/ecommerce/models"
	"github.com/tabrizihamid84/ecommerce/tokens"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

var UserCollection *mongo.Collection = database.UserData(database.Client, "Users")
var ProductCollection *mongo.Collection = database.ProductData(database.Client, "Products")
var Validate = validator.New()

func HashPassword(password string) string {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		log.Panic(err)
	}
	return string(bytes)
}

func VerifyPassword(userPassword, givenPassword string) (bool, string) {
	err := bcrypt.CompareHashAndPassword([]byte(givenPassword), []byte(userPassword))
	valid := true
	msg := ""
	if err != nil {
		msg = "Login or Password is incorrect"
		valid = false
	}
	return valid, msg
}

func SignUp() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		if err := c.BindJSON(&user); err != nil {
			return
		}

		validitionErr := Validate.Struct(user)
		if validitionErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": validitionErr})
			return
		}

		count, err := UserCollection.CountDocuments(ctx, bson.M{"email": user.Email})
		defer cancel()
		if err != nil {
			log.Panic(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err})
			return
		}

		if count > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user already exists"})
			return
		}

		count, err = UserCollection.CountDocuments(ctx, bson.M{"phone": user.Phone})
		defer cancel()
		if err != nil {
			log.Panic(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err})
			return
		}

		if count > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "this phone number is already exists"})
			return
		}

		password := HashPassword(*user.Password)
		user.Password = &password

		user.Created_At, _ = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		user.Updated_At, _ = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
		user.ID = primitive.NewObjectID()
		user.User_ID = user.ID.Hex()

		token, refreshToken, _ := tokens.TokenGenerator(*user.Email, *user.First_Name, *user.Last_Name, *&user.User_ID)

		user.Token = &token
		user.Refresh_Token = &refreshToken
		user.UserCart = make([]models.ProductUser, 0)
		user.Address_Details = make([]models.Address, 0)
		user.Order_Status = make([]models.Order, 0)

		_, inserterr := UserCollection.InsertOne(ctx, user)
		defer cancel()
		if inserterr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "the user didn't created"})
			return
		}
		c.JSON(http.StatusCreated, "Successfully signed in!")

	}
}

func Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		if err := c.BindJSON(&user); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err})
			return
		}

		var founduser models.User

		err := UserCollection.FindOne(ctx, bson.M{"email": user.Email}).Decode(&founduser)
		defer cancel()

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "login or password incorrect"})
			return
		}

		passwordIsValid, msg := VerifyPassword(*user.Password, *founduser.Password)
		defer cancel()

		if !passwordIsValid {
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
			fmt.Println(msg)
			return
		}

		token, refreshToken, _ := tokens.TokenGenerator(*&founduser.Email, *&founduser.First_Name, *&founduser.Last_Name, *&founduser.User_ID)
		defer cancel()

		tokens.UpdateAllTokens(token, refreshToken, founduser.User_ID)

		c.JSON(http.StatusFound, founduser)

	}
}

func ProductViewerAdmin() gin.HandlerFunc {}

func SearchProduct() gin.HandlerFunc {
	return func(c *gin.Context) {
		var productList []models.Product
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		cur, err := ProductCollection.Find(ctx, bson.M{})
		defer cancel()
		if err != nil {
			c.IndentedJSON(http.StatusInternalServerError, "something went wrong, please try after some time")
			return
		}

		err = cur.All(ctx, &productList)
		defer cancel()
		if err != nil {
			log.Println(err)
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		defer cur.Close(ctx)
		if err := cur.Err(); err != nil {
			log.Println(err)
			c.IndentedJSON(400, "invalid")
			return
		}

		defer cancel()
		c.IndentedJSON(200, productList)

	}
}

func SearchProductByQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var searchProducts []models.Product
		queryParam := c.Query("name")

		if queryParam == "" {
			log.Println("query is empty")
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid search index"})
			c.Abort()
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		cur, err := ProductCollection.Find(ctx, bson.M{"product_name": bson.M{"$regex": queryParam}})
		defer cancel()
		if err != nil {
			c.IndentedJSON(http.StatusInternalServerError, "something went wrong while fetching the data")
			return
		}

		err = cur.All(ctx, &searchProducts)
		defer cancel()
		if err != nil {
			log.Println(err)
			c.IndentedJSON(400, "invalid")
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		defer cur.Close(ctx)
		if err := cur.Err(); err != nil {
			log.Println(err)
			c.IndentedJSON(400, "invalid")
			return
		}

		defer cancel()
		c.IndentedJSON(200, searchProducts)

	}
}
