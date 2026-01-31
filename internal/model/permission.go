package model

import (
    "database/sql"
    "github.com/google/uuid"
)

type Permission struct {
    ID          uuid.UUID
    Name        string
    Description sql.NullString
}

type Role struct {
    ID          uuid.UUID
    Name        string
    Description sql.NullString
    Permissions []Permission `gorm:"many2many:role_permissions;"`
}