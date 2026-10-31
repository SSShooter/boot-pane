package i18n

// ptMessages 是葡萄牙语文案（巴西用法）。key 与 enMessages 一一对应。
var ptMessages = map[string]string{
	"app.description": "Iniciador de projetos",

	// Registro de configuração
	"store.projectNotFound": "projeto não encontrado",
	"store.configDir":       "não foi possível localizar o diretório de configuração do usuário: %v",
	"store.mkdir":           "falha ao criar o diretório de configuração: %v",
	"store.readFile":        "falha ao ler %s: %v",
	"store.parseWorkspace":  "%s está malformado: %v",
	"store.badPath":         "caminho inválido: %v",
	"store.dirMissing":      "a pasta não existe: %s",
	"store.emptyIds":        "os ids de projeto e tarefa não podem ficar vazios",
	"store.dirUnreadable":   "a pasta do projeto não existe ou não pode ser acessada",
	"store.repoParse":       "falha ao interpretar %s: %v",
	"store.repoMissing":     "configurado para usar a configuração do repositório, mas o repositório não tem %s",
	"store.repoWrite":       "falha ao gravar %s: %v",
	"store.langUnsupported": "idioma não suportado: %q",

	// Validação da configuração
	"validate.idMissing":          "a tarefa %q não tem id",
	"validate.idDuplicate":        "id de tarefa duplicado",
	"validate.commandMissing":     "nenhum comando de inicialização configurado",
	"validate.cwdMissing":         "a pasta de trabalho não existe: %s",
	"validate.badOpenURL":         "openUrl não é um endereço http(s) válido: %s",
	"validate.depMissing":         "a tarefa %q da dependência não existe",
	"validate.portInvalid":        "waitFor.port não é uma porta válida",
	"validate.portDuplicate":      "a porta %d já é usada pela tarefa %q",
	"validate.logKeywordMissing":  "waitFor.type é log mas nenhuma palavra-chave foi definida",
	"validate.waitTypeUnknown":    "waitFor.type desconhecido: %q",
	"validate.pipelineRefMissing": "o pipeline %q referencia a tarefa %q, que não existe",
	"validate.depCycle":           "ciclo de dependências: %s",

	// Importação e leitura
	"import.noTasks": "a configuração não contém nenhuma tarefa",
	"import.noJSON":  "nenhum JSON encontrado, confirme se a configuração completa foi colada",
	"import.badJSON": "JSON inválido: %v",

	// Inicialização de processos
	"run.alreadyRunning":    "“%s” já está em execução",
	"run.cwdMissing":        "a pasta de trabalho não existe: %s",
	"run.treeFail":          "falha ao criar a árvore de processos: %v",
	"run.spawnFail":         "falha ao iniciar: %v",
	"run.exitedImmediately": "o processo encerrou logo após iniciar (código de saída %d)",
	"run.waitLogTimeout":    "tempo esgotado aguardando o log %q",
	"run.waitTypeUnknown":   "tipo de waitFor desconhecido: %s",
	"run.notImplemented":    "o gerenciamento de processos não está disponível nesta plataforma",
	"process.listener":      "%s (PID %d)",
	"process.pidOnly":       "PID %d",

	// Como abrir
	"host.openWithEmpty":   "o comando de abertura não pode ficar vazio",
	"host.commandNotFound": "comando %q não encontrado, verifique se está instalado e no PATH",

	// Camada de serviços
	"svc.taskNoPage":     "a tarefa %q não tem página para abrir",
	"svc.taskNotFound":   "a tarefa %q não existe",
	"svc.importRepoMode": "este projeto usa a configuração do repositório, volte para a global antes de importar",
	"svc.importFailed":   "a configuração importada tem erros e nada foi gravado",
	"svc.saveFailed":     "a configuração tem erros e não foi salva",

	// Reinício de instância externa
	"restart.noPort":      "esta tarefa não tem porta configurada, então instâncias externas não podem ser detectadas",
	"restart.killFailed":  "falha ao encerrar %s: %v",
	"restart.portStuck":   "%d processos encerrados, mas a porta %d continua em uso",
	"restart.started":     "iniciada",
	"restart.done":        "instância externa encerrada e tarefa reiniciada",
	"restart.portTimeout": "a porta %d não foi liberada em %v",

	// Janela de logs
	"logWindow.title":       "Logs de execução · BootPane",
	"logWindow.stateEmpty":  "o estado do painel de logs está vazio",
	"logWindow.appNotReady": "o aplicativo ainda não foi inicializado",
}
