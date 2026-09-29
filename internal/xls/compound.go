// Контейнер OLE Compound File для готового BIFF8-потока Workbook.
package xls

import (
	"encoding/binary"
	"unicode/utf16"
)

const (
	sectorSize         = 512
	freeSector  uint32 = 0xFFFFFFFF
	endOfChain  uint32 = 0xFFFFFFFE
	fatSector   uint32 = 0xFFFFFFFD
	difatSector uint32 = 0xFFFFFFFC
)

// compoundFile упаковывает готовый Workbook в контейнер Compound File версии 3.
// Возвращает XLS с directory/FAT/DIFAT; малые потоки дополняет до 4096 байт без mini-stream.
func compoundFile(workbook []byte) []byte {
	streamSize := max(len(workbook), 4096)
	streamSectors := (streamSize + sectorSize - 1) / sectorSize
	fatCount, difatCount := 0, 0
	for {
		nextFAT := (streamSectors + 1 + fatCount + difatCount + 127) / 128
		nextDIFAT := 0
		if nextFAT > 109 {
			nextDIFAT = (nextFAT - 109 + 126) / 127
		}
		if nextFAT == fatCount && nextDIFAT == difatCount {
			break
		}
		fatCount, difatCount = nextFAT, nextDIFAT
	}
	directorySector := streamSectors
	fatStart := directorySector + 1
	difatStart := fatStart + fatCount
	totalSectors := difatStart + difatCount
	file := make([]byte, (totalSectors+1)*sectorSize)
	header := file[:sectorSize]
	copy(header, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(header[24:], 0x003E)
	binary.LittleEndian.PutUint16(header[26:], 3)
	binary.LittleEndian.PutUint16(header[28:], 0xFFFE)
	binary.LittleEndian.PutUint16(header[30:], 9)
	binary.LittleEndian.PutUint16(header[32:], 6)
	binary.LittleEndian.PutUint32(header[44:], uint32(fatCount))
	binary.LittleEndian.PutUint32(header[48:], uint32(directorySector))
	binary.LittleEndian.PutUint32(header[56:], 4096)
	binary.LittleEndian.PutUint32(header[60:], endOfChain)
	binary.LittleEndian.PutUint32(header[68:], endOfChain)
	if difatCount > 0 {
		binary.LittleEndian.PutUint32(header[68:], uint32(difatStart))
	}
	binary.LittleEndian.PutUint32(header[72:], uint32(difatCount))
	for i := 0; i < 109; i++ {
		value := freeSector
		if i < fatCount {
			value = uint32(fatStart + i)
		}
		binary.LittleEndian.PutUint32(header[76+4*i:], value)
	}
	copy(file[sectorSize:], workbook)
	directory := file[(directorySector+1)*sectorSize:]
	writeDirectoryEntry(directory[:128], "Root Entry", 5, 1, endOfChain, 0)
	writeDirectoryEntry(directory[128:256], "Workbook", 2, freeSector, 0, uint64(streamSize))
	for i := 2; i < 4; i++ {
		for _, offset := range []int{68, 72, 76} {
			binary.LittleEndian.PutUint32(directory[i*128+offset:], freeSector)
		}
	}
	fat := file[(fatStart+1)*sectorSize : (fatStart+fatCount+1)*sectorSize]
	for sector := 0; sector < len(fat)/4; sector++ {
		value := freeSector
		switch {
		case sector < streamSectors-1:
			value = uint32(sector + 1)
		case sector == streamSectors-1 || sector == directorySector:
			value = endOfChain
		case sector >= fatStart && sector < difatStart:
			value = fatSector
		case sector >= difatStart && sector < totalSectors:
			value = difatSector
		}
		binary.LittleEndian.PutUint32(fat[sector*4:], value)
	}
	for i := 0; i < difatCount; i++ {
		difat := file[(difatStart+i+1)*sectorSize:]
		for j := 0; j < 127; j++ {
			fatIndex := 109 + i*127 + j
			value := freeSector
			if fatIndex < fatCount {
				value = uint32(fatStart + fatIndex)
			}
			binary.LittleEndian.PutUint32(difat[j*4:], value)
		}
		next := endOfChain
		if i+1 < difatCount {
			next = uint32(difatStart + i + 1)
		}
		binary.LittleEndian.PutUint32(difat[508:], next)
	}
	return file
}

// writeDirectoryEntry формирует запись каталога Compound File для корня или потока Workbook.
// Записывает UTF-16 имя, тип, связи дерева, стартовый сектор и размер в переданный 128-байтовый буфер.
func writeDirectoryEntry(entry []byte, name string, kind byte, child, start uint32, size uint64) {
	units := utf16.Encode([]rune(name))
	for i, unit := range units {
		binary.LittleEndian.PutUint16(entry[2*i:], unit)
	}
	binary.LittleEndian.PutUint16(entry[64:], uint16((len(units)+1)*2))
	entry[66], entry[67] = kind, 1 // black node in the directory tree
	binary.LittleEndian.PutUint32(entry[68:], freeSector)
	binary.LittleEndian.PutUint32(entry[72:], freeSector)
	binary.LittleEndian.PutUint32(entry[76:], child)
	binary.LittleEndian.PutUint32(entry[116:], start)
	binary.LittleEndian.PutUint64(entry[120:], size)
}
