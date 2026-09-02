// Package configpatch правит конфиг клиента по отдельным полям.
//
// Правка одного поля гнала клиенту весь CONFIG_TYPE_ALL, поэтому два админа,
// тронувшие разные вещи, затирали друг друга целиком. Патч несёт только
// заполненные поля: устройство их и применяет, потому что в proto у полей есть
// presence, а парсер уважает его давно.
package configpatch

import (
	"errors"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
)

// ErrEmpty - патч без единого заполненного поля менять нечего
var ErrEmpty = errors.New("configpatch: патч пустой")

// Paths - какие поля патч трогает, путями вида xray.settings.remote_dns.
//
// Считается обходом самого патча, а не списком от клиента: клиент мог соврать
// или отстать от схемы, а заполненные поля врать не умеют
func Paths(patch *wingsvpb.Config) []string {
	if patch == nil {
		return nil
	}
	out := make([]string, 0, 8)
	walk(patch.ProtoReflect(), "", &out)
	sort.Strings(out)
	return out
}

func walk(message protoreflect.Message, prefix string, out *[]string) {
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		name := string(field.Name())
		if prefix != "" {
			name = prefix + "." + name
		}
		// Тип и версия конфига едут в каждом патче служебно и правкой не считаются
		if prefix == "" && (field.Name() == "type" || field.Name() == "ver") {
			return true
		}
		// Внутрь вложенного сообщения идём глубже: путь до конкретного поля
		// точнее, чем "тронут раздел xray"
		if field.Kind() == protoreflect.MessageKind && !field.IsList() && !field.IsMap() {
			walk(value.Message(), name, out)
			return true
		}
		*out = append(*out, name)
		return true
	})
}

// Apply вливает патч в хранимый конфиг.
//
// Списки заменяются целиком, а не дописываются: набор профилей или подписок -
// это одно значение, и половина старого рядом с половиной нового даёт мусор
func Apply(stored, patch *wingsvpb.Config) (*wingsvpb.Config, error) {
	if patch == nil {
		return nil, ErrEmpty
	}
	paths := Paths(patch)
	if len(paths) == 0 {
		return nil, ErrEmpty
	}
	if stored == nil {
		stored = &wingsvpb.Config{Ver: 1}
	}
	merged, ok := proto.Clone(stored).(*wingsvpb.Config)
	if !ok {
		return nil, errors.New("configpatch: не тот тип конфига")
	}
	clearReplacedLists(merged.ProtoReflect(), patch.ProtoReflect())
	proto.Merge(merged, patch)
	if merged.Ver == 0 {
		merged.Ver = 1
	}
	// Тип патча - это про сам патч, а не про сохранённый конфиг: в хранилище
	// всегда лежит полная картина
	merged.Type = stored.GetType()
	return merged, nil
}

// clearReplacedLists опустошает в приёмнике те списки, которые патч прислал
// заново: без этого proto.Merge допишет новые элементы к старым
func clearReplacedLists(dst, src protoreflect.Message) {
	src.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsList() || field.IsMap():
			dst.Clear(field)
		case field.Kind() == protoreflect.MessageKind:
			if dst.Has(field) {
				clearReplacedLists(dst.Mutable(field).Message(), value.Message())
			}
		}
		return true
	})
}

// Conflicts - поля, которые патч трогает, а кто-то успел поменять раньше.
//
// Сравнение идёт по путям, а не по всему конфигу: правка соседнего поля - это
// не конфликт, и звать человека разбираться там незачем
func Conflicts(paths []string, touched map[string]int64, since int64) []string {
	if since <= 0 || len(touched) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	for _, path := range paths {
		if version, ok := touched[path]; ok && version > since {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// Touch отмечает пути новой версией. Карта живёт рядом с конфигом и говорит,
// когда каждое поле трогали в последний раз
func Touch(touched map[string]int64, paths []string, version int64) map[string]int64 {
	if touched == nil {
		touched = make(map[string]int64, len(paths))
	}
	for _, path := range paths {
		touched[path] = version
	}
	return touched
}

// Describe - человеческое перечисление тронутого для экрана подтверждения
func Describe(paths []string) string {
	return strings.Join(paths, ", ")
}
