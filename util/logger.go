package util

import (
	"io"
	"log"
	"os"
	"sync"
)

// Logger is a struct that contains two loggers: app and chat
type Logger struct {
	mu          sync.Mutex
	App         *log.Logger
	Chat        *log.Logger
	appLogFile  *os.File
	chatLogFile *os.File
}

// Singleton instance of Logger
var (
	lInstance *Logger
	lOnce     sync.Once
)

// GetLogger returns singleton instance of Logger
func GetLogger() *Logger {
	lOnce.Do(func() {
		lInstance = newLogger()
	})

	return lInstance
}

// newLogger creates a new logger with two log files: app.log and chat.log
func newLogger() *Logger {
	// Check if log folder exists, if not create it
	path := "/var/log/"

	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "./log/"
	} else {
		path = path + "dchat/"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			err := os.Mkdir(path, 0666)
			if err != nil {
				log.Fatal("Cannot create folder:", err)
			}
		}
	}

	appLogFile, err := os.OpenFile(path+"app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Cannot open app.log file:", err)
	}

	chatLogFile, err := os.OpenFile(path+"chat.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Cannot open chat.log file:", err)
	}

	multiWriter := io.MultiWriter(os.Stdout, appLogFile)
	appLog := log.New(multiWriter, "[APP] ", log.LstdFlags)

	multiWriterChat := io.MultiWriter(os.Stdout, chatLogFile)
	chatLog := log.New(multiWriterChat, "[CHAT] ", log.LstdFlags)
	chatLog.SetFlags(0)

	return &Logger{
		App:         appLog,
		Chat:        chatLog,
		appLogFile:  appLogFile,
		chatLogFile: chatLogFile,
	}
}

// Close closes log files
func (l *Logger) Close() {
	l.appLogFile.Close()
	l.chatLogFile.Close()
}
