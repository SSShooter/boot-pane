package i18n

// ruMessages 是俄语文案。key 与 enMessages 一一对应。
var ruMessages = map[string]string{
	"app.description": "Запуск проектов",

	// Реестр конфигурации
	"store.projectNotFound": "проект не найден",
	"store.configDir":       "не удалось определить каталог конфигурации пользователя: %v",
	"store.mkdir":           "не удалось создать каталог конфигурации: %v",
	"store.readFile":        "не удалось прочитать %s: %v",
	"store.parseWorkspace":  "%s содержит ошибку формата: %v",
	"store.badPath":         "недопустимый путь: %v",
	"store.dirMissing":      "каталог не существует: %s",
	"store.emptyIds":        "идентификаторы проекта и задачи не могут быть пустыми",
	"store.dirUnreadable":   "каталог проекта отсутствует или недоступен",
	"store.repoParse":       "не удалось разобрать %s: %v",
	"store.repoMissing":     "включена конфигурация репозитория, но в репозитории нет %s",
	"store.repoWrite":       "не удалось записать %s: %v",
	"store.langUnsupported": "неподдерживаемый язык: %q",

	// Проверка конфигурации
	"validate.idMissing":          "у задачи %q отсутствует id",
	"validate.idDuplicate":        "повторяющийся id задачи",
	"validate.commandMissing":     "не задана команда запуска",
	"validate.cwdMissing":         "рабочий каталог не существует: %s",
	"validate.badOpenURL":         "openUrl не является допустимым адресом http(s): %s",
	"validate.depMissing":         "задача-зависимость %q не существует",
	"validate.portInvalid":        "waitFor.port не является допустимым портом",
	"validate.portDuplicate":      "порт %d уже занят задачей %q",
	"validate.logKeywordMissing":  "waitFor.type равно log, но ключевое слово не задано",
	"validate.waitTypeUnknown":    "неизвестный waitFor.type: %q",
	"validate.pipelineRefMissing": "конвейер %q ссылается на несуществующую задачу %q",
	"validate.depCycle":           "цикл зависимостей: %s",

	// Импорт и разбор
	"import.noTasks": "в конфигурации нет ни одной задачи",
	"import.noJSON":  "JSON не найден, проверьте, что вставлена вся конфигурация",
	"import.badJSON": "ошибка формата JSON: %v",

	// Запуск процессов
	"run.alreadyRunning":    "«%s» уже запущена",
	"run.cwdMissing":        "рабочий каталог не существует: %s",
	"run.treeFail":          "не удалось создать дерево процессов: %v",
	"run.spawnFail":         "не удалось запустить: %v",
	"run.exitedImmediately": "процесс завершился сразу после запуска (код выхода %d)",
	"run.waitLogTimeout":    "превышено время ожидания строки %q в логе",
	"run.waitTypeUnknown":   "неизвестный тип waitFor: %s",
	"run.notImplemented":    "управление процессами не реализовано для этой платформы",
	"process.listener":      "%s (PID %d)",
	"process.pidOnly":       "PID %d",

	// Открытие каталога
	"host.openWithEmpty":   "команда открытия не может быть пустой",
	"host.commandNotFound": "команда %q не найдена, проверьте, что она установлена и есть в PATH",

	// Сервисный уровень
	"svc.taskNoPage":     "у задачи %q нет страницы для открытия",
	"svc.taskNotFound":   "задача %q не существует",
	"svc.importRepoMode": "проект использует конфигурацию репозитория — перед импортом переключитесь на глобальную",
	"svc.importFailed":   "в импортированной конфигурации есть ошибки, запись отменена",
	"svc.saveFailed":     "в конфигурации есть ошибки, сохранение отменено",

	// Перезапуск внешнего экземпляра
	"restart.noPort":      "у задачи не настроен порт, внешний экземпляр не определить",
	"restart.killFailed":  "не удалось завершить %s: %v",
	"restart.portStuck":   "завершено процессов: %d, но порт %d всё ещё занят",
	"restart.started":     "запущено",
	"restart.done":        "внешний экземпляр завершён, задача перезапущена",
	"restart.portTimeout": "порт %d не освободился за %v",

	// Отдельное окно логов
	"logWindow.title":       "Журнал запуска · BootPane",
	"logWindow.stateEmpty":  "состояние панели логов пустое",
	"logWindow.appNotReady": "приложение ещё не инициализировано",
}
