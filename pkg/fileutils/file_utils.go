package file_utils

import (
	"log"
	"os"
)

func WriteToFile(fileName string, data []string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	log.Printf("Writing log data to the file %s", fileName)
	for _, l := range data {
		_, err = f.WriteString(l)
		if err != nil {
			log.Default().Printf("Failed to write some data to file name %s: %v", fileName, err)
			continue
		}
	}
	return nil
}
