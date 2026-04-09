// rest-api demonstrates RestController with JSON binding and path parameters.
//
// Routes:
//   GET  /api/users         list all users
//   GET  /api/users/{id}    get a user by ID
//   POST /api/users         create a user
package main

import (
	"context"
	"log"

	glk "github.com/hansir-hsj/GoLiteKit"
)

// ---- models ----------------------------------------------------------------

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// ---- list users ------------------------------------------------------------

type ListUsersController struct {
	glk.RestController[glk.NoBody]
}

func (c *ListUsersController) Serve(ctx context.Context) error {
	users := []User{
		{ID: 1, Name: "alice", Age: 30},
		{ID: 2, Name: "bob", Age: 25},
	}
	c.ServeData(ctx, users)
	return nil
}

// ---- get user by ID --------------------------------------------------------

type GetUserController struct {
	glk.RestController[glk.NoBody]
}

func (c *GetUserController) Serve(ctx context.Context) error {
	id := c.PathValueInt("id", 0)
	if id <= 0 {
		return c.BadRequest("invalid id", nil)
	}
	user := User{ID: id, Name: "alice", Age: 30}
	c.ServeData(ctx, user)
	return nil
}

// ---- create user -----------------------------------------------------------

type CreateUserReq struct {
	Name string `json:"name" form:"name"`
	Age  int    `json:"age"  form:"age"`
}

type CreateUserController struct {
	glk.RestController[CreateUserReq]
}

func (c *CreateUserController) Serve(ctx context.Context) error {
	req := c.GetRequest()
	if req.Name == "" {
		return c.BadRequest("name is required", nil)
	}
	created := User{ID: 3, Name: req.Name, Age: req.Age}
	c.ServeData(ctx, created)
	return nil
}

// ---- main ------------------------------------------------------------------

func main() {
	app := glk.NewApp()

	api := app.Group("/api")
	api.GET("/users", &ListUsersController{})
	api.GET("/users/{id}", &GetUserController{})
	api.POST("/users", &CreateUserController{})

	log.Println("listening on :8080")
	if err := app.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
