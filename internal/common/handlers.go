package common

import (
	log "github.com/sirupsen/logrus"
)

func LogError(err error, fatal bool) {
	if err != nil {
		log.Error(err)
	}

	if fatal {
		panic(err)
	}
}

func LogInfo(info string) {
	log.Info(info)
}
