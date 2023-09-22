package conf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// valid passphrase must be between 10 and 79 characters
var (
	emptyPassphrase    = ""
	tooShortPassphrase = "too short"
	tooLongPassphrase  = "Corre l'anno 2030" +
		"E mi ritrovo che di anni quasi ne ho sessanta " +
		"Il mio pizzetto è grigio, e di capelli sono senza"
	validPassphrase = "Wherever you go, there you are."
)

func TestCheckSrtPassphrase(t *testing.T) {
	which := "read"
	pathName := "example"
	err1 := checkSrtPassphrase(emptyPassphrase, which, pathName)
	require.NotNil(t, err1)

	err2 := checkSrtPassphrase(tooShortPassphrase, which, pathName)
	require.NotNil(t, err2)

	err3 := checkSrtPassphrase(tooLongPassphrase, which, pathName)
	require.NotNil(t, err3)

	err4 := checkSrtPassphrase(validPassphrase, which, pathName)
	require.Nil(t, err4)
}

func TestCheckReadSrtPassphrase(t *testing.T) {
	pathName := "readPath"
	pathConf1 := PathConf{ReadSRTPassphrase: emptyPassphrase}
	err1 := pathConf1.CheckReadSrtPassphrase(pathName)
	require.NotNil(t, err1)

	pathConf2 := PathConf{ReadSRTPassphrase: tooShortPassphrase}
	err2 := pathConf2.CheckReadSrtPassphrase(pathName)
	require.NotNil(t, err2)

	pathConf3 := PathConf{ReadSRTPassphrase: tooLongPassphrase}
	err3 := pathConf3.CheckReadSrtPassphrase(pathName)
	require.NotNil(t, err3)

	pathConf4 := PathConf{ReadSRTPassphrase: validPassphrase}
	err4 := pathConf4.CheckReadSrtPassphrase(pathName)
	require.Nil(t, err4)
}

func TestCheckPublishSrtPassphrase(t *testing.T) {
	pathName := "publishPath"
	pathConf1 := PathConf{PublishSRTPassphrase: emptyPassphrase}
	err1 := pathConf1.CheckPublishSrtPassphrase(pathName)
	require.NotNil(t, err1)

	pathConf2 := PathConf{PublishSRTPassphrase: tooShortPassphrase}
	err2 := pathConf2.CheckPublishSrtPassphrase(pathName)
	require.NotNil(t, err2)

	pathConf3 := PathConf{PublishSRTPassphrase: tooLongPassphrase}
	err3 := pathConf3.CheckPublishSrtPassphrase(pathName)
	require.NotNil(t, err3)

	pathConf4 := PathConf{PublishSRTPassphrase: validPassphrase}
	err4 := pathConf4.CheckPublishSrtPassphrase(pathName)
	require.Nil(t, err4)
}
