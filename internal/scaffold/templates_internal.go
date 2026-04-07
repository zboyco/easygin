package scaffold

func internalTemplates() []projectFile {
	return []projectFile{
		textFile("internal/constants/types/example.go", exampleTypeTemplate),
		textFile("internal/controllers/user/function.go", userControllerTemplate),
		textFile("internal/controllers/role/function.go", roleControllerTemplate),
		textFile("internal/models/db.go", dbModelTemplate),
		textFile("internal/models/user.go", userModelTemplate),
		textFile("internal/models/role.go", roleModelTemplate),
	}
}

const exampleTypeTemplate = `package types

type Example string

const (
	ExampleBasic Example = "basic"
	ExampleRole  Example = "role"
	ExampleUser  Example = "user"
)

var Examples = []Example{
	ExampleBasic,
	ExampleRole,
	ExampleUser,
}
`

const userControllerTemplate = `package user

import "__MODULE__/internal/models"

func List() []models.User {
	return []models.User{
		{
			ID:     1,
			Name:   "alice",
			Email:  "alice@example.com",
			RoleID: 1,
		},
		{
			ID:     2,
			Name:   "bob",
			Email:  "bob@example.com",
			RoleID: 2,
		},
	}
}

func Get(id int64) *models.User {
	return &models.User{
		ID:     id,
		Name:   "alice",
		Email:  "alice@example.com",
		RoleID: 1,
	}
}

func Create(name, email string, roleID int64) *models.User {
	return &models.User{
		ID:     1,
		Name:   name,
		Email:  email,
		RoleID: roleID,
	}
}

func Update(id int64, name, email string, roleID int64) *models.User {
	return &models.User{
		ID:     id,
		Name:   name,
		Email:  email,
		RoleID: roleID,
	}
}
`

const roleControllerTemplate = `package role

import "__MODULE__/internal/models"

func List() []models.Role {
	return []models.Role{
		{
			ID:   1,
			Name: "admin",
			Desc: "system administrator",
		},
		{
			ID:   2,
			Name: "editor",
			Desc: "content editor",
		},
	}
}

func Get(id int64) *models.Role {
	return &models.Role{
		ID:   id,
		Name: "admin",
		Desc: "system administrator",
	}
}

func Create(name, desc string, parentID int64) *models.Role {
	return &models.Role{
		ID:       1,
		Name:     name,
		Desc:     desc,
		ParentID: parentID,
	}
}

func Update(id int64, name, desc string, parentID int64) *models.Role {
	return &models.Role{
		ID:       id,
		Name:     name,
		Desc:     desc,
		ParentID: parentID,
	}
}
`

const dbModelTemplate = `package models

type DB struct {
	Driver string ` + "`json:\"driver\"`" + `
	DSN    string ` + "`json:\"dsn\"`" + `
}

func NewDB(driver, dsn string) *DB {
	return &DB{
		Driver: driver,
		DSN:    dsn,
	}
}
`

const userModelTemplate = `package models

type User struct {
	ID     int64  ` + "`json:\"id\"`" + `
	Name   string ` + "`json:\"name\"`" + `
	Email  string ` + "`json:\"email\"`" + `
	RoleID int64  ` + "`json:\"roleId\"`" + `
}
`

const roleModelTemplate = `package models

type Role struct {
	ID       int64  ` + "`json:\"id\"`" + `
	Name     string ` + "`json:\"name\"`" + `
	Desc     string ` + "`json:\"desc,omitempty\"`" + `
	ParentID int64  ` + "`json:\"parentId,omitempty\"`" + `
}
`
