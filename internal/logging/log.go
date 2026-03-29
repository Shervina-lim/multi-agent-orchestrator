package logging

import (
	"log"
	"os"
)

var debugEnabled bool

func init() {
	if v := os.Getenv("DEBUG"); v == "1" || v == "true" {
		debugEnabled = true
	}
}

func Debugf(format string, v ...interface{}) {
	if debugEnabled {
		log.Printf("DEBUG: "+format, v...)
	}
}

func Infof(format string, v ...interface{}) {
	log.Printf(format, v...)
}
