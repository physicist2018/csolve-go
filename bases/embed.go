// Package bases содержит встроенные таблицы сечений, которые компилируются
// в бинарник csolve.
//
// Каждая таблица хранится рядом со своим текстовым исходником в сжатом виде
// (data.txt.gz) и встраивается через embed. Текстовые data.txt остаются
// источником правды: после их изменения нужно пересобрать .gz командой
//
//	go generate ./bases        (или make generate)
//
// Генератор детерминирован, поэтому повторная сборка не даёт диффов.
package bases

import "embed"

//go:generate go run ../internal/gen -dir .

// FS — встроенные наборы таблиц сечений (только каталоги average): sphere
// (base_sph_25), spheroid (base_sphrd_25), agglomerate (large_aggl_100) и
// agglomerate-cs (large_aggl_100_cs).
//
//go:embed base_sph_25/average/*/*.gz
//go:embed base_sphrd_25/average/*/*.gz
//go:embed large_aggl_100/average/*/*.gz
//go:embed large_aggl_100_cs/average/*/*.gz
var FS embed.FS
