package file_utils

import (
	"log"
	"os"
)

func WriteToFile(fileName string, data string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}

	defer f.Close()
	log.Printf("Writing log data to the file %s", fileName)
	_, err = f.WriteString(data)
	return err
}
