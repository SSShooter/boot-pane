package i18n

// catalog 汇总各语言的文案表。新增语言时在这里注册，其余代码不用动。
var catalog = map[Lang]map[string]string{
	ZH: zhMessages,
	EN: enMessages,
	PT: ptMessages,
	RU: ruMessages,
}
