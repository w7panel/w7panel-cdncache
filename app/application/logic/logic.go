package logic

import (
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"gorm.io/gorm"
)

type logic struct {
}

func (l logic) GetDefaultDb() *gorm.DB {
	db, _ := facade.GetDbFactory().Channel("default")
	return db
}
