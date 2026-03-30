package vtui

// Стандартные идентификаторы команд, общие для всего фреймворка.
const (
	CmValid         = iota // 0
	CmQuit                 // Выход из приложения
	CmOK                   // Подтверждение (ОК)
	CmCancel               // Отмена
	CmYes                  // Да
	CmNo                   // Нет
	CmDefault              // Действие по умолчанию (Enter в диалоге)
	CmClose                // Закрыть окно
	CmZoom                 // Развернуть/свернуть окно (F5)
	CmResize               // Изменить размер
	CmNext                 // Следующее окно
	CmPrev                 // Предыдущее окно
	CmHelp                 // Вызов справки (F1)
	CmReceivedFocus        // Фрейм получил focus
	CmReleasedFocus        // Фрейм потерял focus

	// Application level standard commands
	CmCopy   = 100
	CmMove   = 101
	CmDelete = 102
	CmView   = 103
	CmEdit   = 104
	CmSearch = 105
	CmBackground = 106
	CmMkDir      = 107
	CmLeftMedium    = 200
	CmLeftDetailed  = 201
	CmRightMedium   = 202
	CmRightDetailed = 203
)// CommandSet is a collection of command IDs, used to enable/disable groups of actions.
type CommandSet struct {
	mask map[int]bool
}

func NewCommandSet() CommandSet {
	return CommandSet{mask: make(map[int]bool)}
}

func (cs *CommandSet) Disable(cmd int) {
	if cs.mask == nil { cs.mask = make(map[int]bool) }
	cs.mask[cmd] = true
}

func (cs *CommandSet) Enable(cmd int) {
	if cs.mask == nil { return }
	delete(cs.mask, cmd)
}

func (cs *CommandSet) IsDisabled(cmd int) bool {
	if cs.mask == nil { return false }
	return cs.mask[cmd]
}

func (cs *CommandSet) Clear() {
	cs.mask = make(map[int]bool)
}
