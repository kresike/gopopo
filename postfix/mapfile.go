package postfix

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"log"
)

// Load a map file into a memorymap
func Load(filename string, l *log.Logger) *MemoryMap {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Println("opening file: ", err.Error())
		return nil
	}
	defer f.Close()
	c := 0
	s := bufio.NewScanner(f)
	res := NewMemoryMap()
	for s.Scan() {
		c++
		str := s.Text()
		if strings.HasPrefix(str, "#") {
			l.Printf("ignoring comment at %s:%d (%s)\n", filename, c, str)
			continue
		}
		t := strings.Fields(str)
		if len(t) != 2 {
			l.Printf("cannot parse file content of %s at line %d: %s\n", filename, c, t)
			continue
		}
		res.Add(t[0], t[1])
	}
	return res
}
