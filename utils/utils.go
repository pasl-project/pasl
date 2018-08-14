/*
PASL - Personalized Accounts & Secure Ledger

Copyright (C) 2018 PASL Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package utils

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"
)

func MaxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func MinInt64(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func MaxInt32(a int32, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func MinInt32(a int32, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func MaxUint32(a uint32, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func MinUint32(a uint32, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func MaxUint64(a uint64, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func TimeTrack(start time.Time, format string) {
	elapsed := time.Since(start)
	Tracef(format, elapsed)
}

func getHomeDir() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir, nil
	}
	return "", errors.New("Failed to obtain user's home dir")
}

func getDataDir() (string, error) {
	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "pasl"), nil
	} else if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "pasl"), nil
	}
	return filepath.Join(home, ".pasl"), nil
}

func CreateDataDir() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0600)
		if err != nil {
			return "", err
		}
	}

	return dir, nil
}

func formatf(format string, a ...interface{}) string {
	pc := make([]uintptr, 15)
	n := runtime.Callers(3, pc)
	frames := runtime.CallersFrames(pc[:n])
	frame, _ := frames.Next()
	return fmt.Sprintf("%s %s %s:%d%s\n", time.Now().UTC().Format("15:04:05"), fmt.Sprintf(format, a...), filepath.Base(frame.File), frame.Line, filepath.Ext(frame.Function))
}

func Tracef(format string, a ...interface{}) {
	fmt.Printf(formatf(format, a...))
}

func Panicf(format string, a ...interface{}) {
	panic(formatf(format, a...))
}
