package tui

// Language represents the UI language.
type Lang string

const (
	LangEN Lang = "en"
	LangKO Lang = "ko"
)

// T returns the translated string for the given key.
var currentLang = LangEN

func T(key string) string {
	if t, ok := translations[currentLang][key]; ok {
		return t
	}
	if t, ok := translations[LangEN][key]; ok {
		return t
	}
	return key
}

func SetLang(l Lang) { currentLang = l }

var translations = map[Lang]map[string]string{
	LangEN: {
		// Navigation
		"nav.keycenter": "Keycenter",
		"nav.vaults":    "Vaults",
		"nav.settings":  "Settings",
		"nav.audit":     "Audit",
		"nav.plugins":   "Plugins",
		"nav.functions": "Functions",

		// Login
		"login.title":         "VeilKey VaultCenter",
		"login.connecting":    "Connecting...",
		"login.unlocking":     "Unlocking server...",
		"login.authenticating": "Authenticating...",
		"login.server_locked": "Server is locked",
		"login.server_unlocked": "Server unlocked",
		"login.master_key":    "Master Key",
		"login.totp_code":     "TOTP Code",
		"login.admin_password": "Admin Password",
		"login.help_auth":     "enter submit  tab switch method  q quit",
		"login.help_unlock":   "enter submit  q quit",

		// KeyCenter
		"kc.title":   "Temp Refs",
		"kc.create":  "Create Temp Ref",
		"kc.name":    "Name",
		"kc.value":   "Value",
		"kc.promote": "Promote to Vault",
		"kc.empty":   "No temp refs.",
		"kc.offline": "Cannot reach VaultCenter",
		"kc.detail":  "Ref Detail",

		// Vaults
		"vaults.title":     "Vaults",
		"vaults.secrets":   "Secrets",
		"vaults.agents":    "Agents",
		"vaults.catalog":   "Catalog",
		"vaults.empty":     "No vaults.",
		"vaults.loading":   "Loading...",
		"vaults.no_agents": "No agents.",
		"vaults.no_secrets": "No secrets.",
		"vaults.no_matches": "No matches.",
		"vaults.no_catalog": "No secrets in catalog.",
		"vaults.search":    "Search secrets",
		"vaults.search_catalog": "Search catalog",
		"vaults.new_secret": "New Secret",
		"vaults.edit_secret": "Edit",
		"vaults.delete_confirm": "Delete secret?",

		// Functions
		"fn.title":       "Functions",
		"fn.bindings":    "Bindings",
		"fn.empty":       "No global functions.",
		"fn.no_bindings": "No bindings.",

		// Settings
		"settings.title":    "Settings",
		"settings.status":   "Status",
		"settings.security": "Security",
		"settings.tokens":   "Tokens",
		"settings.configs":  "Configs",
		"settings.no_tokens": "No registration tokens.",
		"settings.no_configs": "No configs.",
		"settings.new_token": "New Registration Token",

		// Audit
		"audit.title": "Audit Events",
		"audit.empty": "No audit events.",

		// Plugins
		"plugins.title": "Plugins",
		"plugins.empty": "No plugins installed.",

		// Common
		"common.loading": "Loading...",
		"common.offline": "Cannot reach VaultCenter",
		"common.confirm": "Are you sure? (y/n)",
		"common.yes":     "Yes",
		"common.no":      "No",
		"common.cancel":  "Cancel",
		"common.save":    "Save",
		"common.delete":  "Delete",
		"common.back":    "Back",
		"common.refresh": "Refresh",
		"common.search":  "Search:",
		"common.no_delete": "Temp refs expire automatically — manual delete not supported.",
		"common.decrypting": "Decrypting...",
		"common.promoting":  "Promoting...",
		"common.running":    "Running...",
		"common.loading_vaults": "Loading vaults...",
		"common.loading_meta":   "Loading metadata...",
		"common.select_vault":   "Select target vault:",

		// Keys help
		"help.tab":   "tab switch",
		"help.enter": "enter select",
		"help.esc":   "esc back",
		"help.r":     "r refresh",
		"help.c":     "c create",
		"help.d":     "d delete",
		"help.p":     "p promote",
		"help.s":     "s search",
		"help.q":     "q quit",
		"help.slash": "/ search",

		// Language
		"settings.lang":        "Language",
		"settings.lang_toggle": "l toggle language (EN/KO)",
	},
	LangKO: {
		"nav.keycenter": "키센터",
		"nav.vaults":    "볼트",
		"nav.settings":  "설정",
		"nav.audit":     "감사 로그",
		"nav.plugins":   "플러그인",
		"nav.functions": "함수",

		"login.title":         "VeilKey VaultCenter",
		"login.connecting":    "연결 중...",
		"login.unlocking":     "서버 잠금 해제 중...",
		"login.authenticating": "인증 중...",
		"login.server_locked": "서버가 잠겨 있습니다",
		"login.server_unlocked": "서버 잠금 해제됨",
		"login.master_key":    "마스터 키",
		"login.totp_code":     "TOTP 코드",
		"login.admin_password": "관리자 비밀번호",
		"login.help_auth":     "enter 제출  tab 전환  q 종료",
		"login.help_unlock":   "enter 제출  q 종료",

		"kc.title":   "임시 참조",
		"kc.create":  "임시 참조 생성",
		"kc.name":    "이름",
		"kc.value":   "값",
		"kc.promote": "볼트로 승격",
		"kc.empty":   "임시 참조가 없습니다.",
		"kc.offline": "VaultCenter에 연결할 수 없습니다",
		"kc.detail":  "참조 상세",

		"vaults.title":     "볼트",
		"vaults.secrets":   "시크릿",
		"vaults.agents":    "에이전트",
		"vaults.catalog":   "카탈로그",
		"vaults.empty":     "볼트가 없습니다.",
		"vaults.loading":   "로딩 중...",
		"vaults.no_agents": "에이전트가 없습니다.",
		"vaults.no_secrets": "시크릿이 없습니다.",
		"vaults.no_matches": "일치하는 항목이 없습니다.",
		"vaults.no_catalog": "카탈로그에 시크릿이 없습니다.",
		"vaults.search":    "시크릿 검색",
		"vaults.search_catalog": "카탈로그 검색",
		"vaults.new_secret": "시크릿 생성",
		"vaults.edit_secret": "편집",
		"vaults.delete_confirm": "시크릿을 삭제하시겠습니까?",

		"fn.title":       "함수",
		"fn.bindings":    "바인딩",
		"fn.empty":       "전역 함수가 없습니다.",
		"fn.no_bindings": "바인딩이 없습니다.",

		"settings.title":    "설정",
		"settings.status":   "상태",
		"settings.security": "보안",
		"settings.tokens":   "토큰",
		"settings.configs":  "설정값",
		"settings.no_tokens": "등록 토큰이 없습니다.",
		"settings.no_configs": "설정값이 없습니다.",
		"settings.new_token": "새 등록 토큰",

		"audit.title": "감사 이벤트",
		"audit.empty": "감사 항목이 없습니다.",

		"plugins.title": "플러그인",
		"plugins.empty": "설치된 플러그인이 없습니다.",

		"common.loading":  "로딩 중...",
		"common.offline":  "VaultCenter에 연결할 수 없습니다",
		"common.confirm":  "확인하시겠습니까? (y/n)",
		"common.yes":      "예",
		"common.no":       "아니오",
		"common.cancel":   "취소",
		"common.save":     "저장",
		"common.delete":   "삭제",
		"common.back":     "뒤로",
		"common.refresh":  "새로고침",
		"common.search":   "검색:",
		"common.no_delete": "임시 참조는 자동으로 만료됩니다 — 수동 삭제를 지원하지 않습니다.",
		"common.decrypting": "복호화 중...",
		"common.promoting":  "승격 중...",
		"common.running":    "실행 중...",
		"common.loading_vaults": "볼트 로딩 중...",
		"common.loading_meta":   "메타데이터 로딩 중...",
		"common.select_vault":   "대상 볼트 선택:",

		"help.tab":   "tab 전환",
		"help.enter": "enter 선택",
		"help.esc":   "esc 뒤로",
		"help.r":     "r 새로고침",
		"help.c":     "c 생성",
		"help.d":     "d 삭제",
		"help.p":     "p 승격",
		"help.s":     "s 검색",
		"help.q":     "q 종료",
		"help.slash": "/ 검색",

		"settings.lang":        "언어",
		"settings.lang_toggle": "l 언어 전환 (EN/KO)",
	},
}
