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

