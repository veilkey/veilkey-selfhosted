package api

// installGateTemplate holds the full install wizard HTML/CSS/JS.
// Separated from api.go for maintainability — the template is ~800 lines.
// Format verbs: %s flow, %s lastStage, %s targetNodePlaceholder, %s targetVMIDPlaceholder, %s sessionJSON, %s runtimeJSON
const installGateTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Install Gate</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,system-ui,sans-serif;background:radial-gradient(circle at top,#16315f 0,#0f172a 42%%,#0a1020 100%%);color:#f9fafb;min-height:100vh;padding:24px}
.shell{max-width:1120px;margin:0 auto;display:grid;grid-template-columns:1.15fr .85fr;gap:18px}
.card{width:100%%;background:rgba(14,23,43,.9);border:1px solid rgba(148,163,184,.16);border-radius:24px;padding:28px;box-shadow:0 18px 60px rgba(0,0,0,.34);backdrop-filter:blur(12px)}
.eyebrow{font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#93c5fd}
h1{font-size:34px;line-height:1.1;margin-top:8px;margin-bottom:10px}
p{color:#cbd5e1;line-height:1.6}
.row{display:flex;flex-wrap:wrap;gap:10px;margin-top:18px}
.chip{display:inline-block;padding:7px 10px;border-radius:999px;background:#0b1220;border:1px solid #334155;color:#e5e7eb;font-size:13px}
.lang-switch{margin-top:18px;display:flex;gap:8px}
.lang-switch button{border:1px solid #334155;background:#101827;color:#cbd5e1;border-radius:999px;padding:8px 12px;cursor:pointer}
.lang-switch button.active{background:#38bdf8;color:#082f49;border-color:#38bdf8}
.hero{display:grid;grid-template-columns:1fr 220px;gap:18px;align-items:start}
.hero-panel{background:linear-gradient(180deg,rgba(56,189,248,.13),rgba(15,23,42,.08));border:1px solid rgba(56,189,248,.18);border-radius:20px;padding:16px}
.hero-panel strong{display:block;font-size:14px}
.hero-panel span{display:block;margin-top:6px;color:#cbd5e1;font-size:13px;line-height:1.5}
.companion-note{margin-top:14px;padding:12px 14px;border-radius:14px;background:rgba(15,23,42,.72);border:1px solid rgba(148,163,184,.18)}
.companion-note strong{display:block;font-size:13px}
.companion-note span{display:block;margin-top:6px;color:#cbd5e1;font-size:12px;line-height:1.5}
.section-title{font-size:15px;font-weight:700;margin-top:22px;margin-bottom:10px}
.field-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.field{display:flex;flex-direction:column;gap:6px}
.field label{font-size:12px;color:#cbd5e1}
.field input,.field select{width:100%%;background:#0b1220;color:#f8fafc;border:1px solid #334155;border-radius:12px;padding:12px}
.field.full{grid-column:1/-1}
.field small{color:#94a3b8;font-size:12px}
.stack{display:grid;gap:16px}
.step-block{margin-top:18px;padding:18px;border-radius:18px;background:#0b1220;border:1px solid #334155}
.step-kicker{font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#93c5fd;margin-bottom:10px}
.step-block h2{font-size:20px;margin-bottom:8px}
.step-block p{font-size:14px}
.option-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;margin-top:16px}
.option-card{display:block;padding:16px;border:1px solid #334155;border-radius:18px;background:#101827;cursor:pointer}
.option-card.active{border-color:#38bdf8;background:rgba(56,189,248,.12)}
.option-card input{margin-right:8px}
.option-card strong{display:block;font-size:15px}
.option-card span{display:block;margin-top:6px;color:#cbd5e1;font-size:13px;line-height:1.5}
.hidden{display:none !important}
.summary{margin-top:16px;padding:14px;border-radius:16px;background:#0b1220;border:1px solid #334155}
.summary strong{display:block;font-size:14px}
.summary ul{margin-top:8px;padding-left:18px;color:#dbeafe}
.summary li{margin:6px 0}
.actions{display:flex;flex-wrap:wrap;gap:12px;margin-top:18px}
.btn{border:0;border-radius:12px;padding:12px 16px;font-weight:700;cursor:pointer}
.btn-primary{background:#38bdf8;color:#082f49}
.btn-soft{background:#1e293b;color:#e2e8f0;border:1px solid #334155}
.note{margin-top:12px;font-size:12px;color:#94a3b8}
.status-box{margin-top:18px;padding:14px;border-radius:16px;background:#0b1220;border:1px solid #334155}
.status-box pre{white-space:pre-wrap;font-size:12px;line-height:1.5;color:#dbeafe}
.links{margin-top:22px;display:flex;flex-wrap:wrap;gap:12px}
.links a{color:#93c5fd;text-decoration:none}
details{margin-top:18px;border:1px solid #334155;border-radius:16px;background:#0b1220}
summary{cursor:pointer;list-style:none;padding:14px 16px;font-weight:700}
summary::-webkit-details-marker{display:none}
.advanced{padding:0 16px 16px}
.advanced-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.advanced .field input,.advanced .field select{background:#111827}
@media (max-width: 920px){.shell{grid-template-columns:1fr}.field-grid,.advanced-grid,.hero{grid-template-columns:1fr}}
</style></head><body>
<div class="shell">
<div class="card">
<div class="eyebrow">Guided First Install</div>
<h1 id="title">VeilKey 첫 설치 시작</h1>
<p id="subtitle">먼저 설치 방식을 고르고, 다음 단계에서 그 방식에 맞는 최소 입력만 보여줍니다.</p>
<div class="row">
<span class="chip">flow: %s</span>
<span class="chip">last_stage: %s</span>
<span class="chip" id="target-chip">target: linux-host</span>
</div>
<div class="lang-switch">
<button type="button" id="lang-ko" class="active">한국어</button>
<button type="button" id="lang-en">English</button>
</div>
<div class="hero">
<div>
<div class="stack">
<div class="step-block">
<div class="step-kicker" id="step-1-kicker">Step 1</div>
<h2 id="quick-title">어디에 설치할지 먼저 고릅니다</h2>
<p id="step-1-copy">여기서는 설치 방식만 정합니다. Proxmox 정보나 루트 경로는 다음 단계에서 필요한 경우에만 나옵니다.</p>
<div class="option-grid">
<label class="option-card" id="option-linux-host">
<input type="radio" name="target_mode_choice" value="linux-host" checked>
<strong id="linux-option-title">일반 Linux 서버</strong>
<span id="linux-option-copy">이 서버나 다른 Linux 서버에 직접 설치할 때 사용합니다. 기본 경로입니다.</span>
</label>
<label class="option-card" id="option-lxc-allinone">
<input type="radio" name="target_mode_choice" value="lxc-allinone">
<strong id="lxc-option-title">Proxmox LXC 올인원</strong>
<span id="lxc-option-copy">새 LXC를 만들거나 기존 LXC에 all-in-one 런타임을 설치할 때만 선택합니다.</span>
</label>
</div>
<input id="target_mode" type="hidden" value="linux-host">
</div>
<div class="step-block">
<div class="step-kicker" id="step-2-kicker">Step 2</div>
<h2 id="step-2-title">선택한 방식에 맞는 정보만 입력합니다</h2>
<p id="step-2-copy">Linux 설치는 도메인과 설치 루트만, Proxmox LXC 설치는 node와 VMID만 보입니다.</p>
<div id="linux-fields" class="field-grid">
<div class="field">
<label for="public_host" id="public-host-label">접속 주소 또는 도메인</label>
<input id="public_host" placeholder="1234.60.internal.kr">
<small id="public-host-help">설치 후 사용자가 접속할 예상 공개 주소입니다. 현재 wizard 주소는 자동 저장하지 않습니다.</small>
</div>
<div class="field">
<label for="tls_mode" id="tls-mode-label">TLS 방식</label>
<select id="tls_mode">
<option value="later">나중에 설정</option>
<option value="existing">기존 인증서 사용</option>
</select>
<small id="tls-mode-help">처음 검증은 HTTP 기준으로 시작하고, 운영 전 TLS를 붙일 수 있습니다.</small>
</div>
<div class="field full">
<label for="install_root" id="install-root-label">설치 대상 루트</label>
<input id="install_root" placeholder="/">
<small id="install-root-help">리눅스 서버 직접 설치일 때만 사용합니다. 현재 서버의 live root면 추가 확인이 필요합니다.</small>
</div>
<div class="field full">
<label for="localvault_url" id="localvault-label">기존 LocalVault URL (선택)</label>
<input id="localvault_url" placeholder="https://localvault.example.internal">
<small id="localvault-help">새 LocalVault를 같이 설치하지 않고, 기존 LocalVault를 연결할 때만 입력합니다.</small>
</div>
<div class="field full">
<label><input type="checkbox" id="confirm-dangerous-root"> <span id="confirm-dangerous-root-label">이 서버의 루트(/)에 직접 설치하는 위험을 이해했고, 필요한 경우에만 실행합니다.</span></label>
</div>
</div>
<div id="lxc-fields" class="field-grid hidden">
<div class="field">
<label for="lxc_mode" id="lxc-mode-label">LXC 방식</label>
<select id="lxc_mode">
<option value="new">새 LXC 생성</option>
<option value="existing">기존 LXC 사용</option>
</select>
<small id="lxc-mode-help">Proxmox 대상 정보가 필요할 때만 이 단계가 열립니다.</small>
</div>
<div class="field">
<label for="target_node" id="target-node-label">Proxmox node</label>
<input id="target_node" placeholder="%s">
</div>
<div class="field">
<label for="target_vmid" id="target-vmid-label">LXC VMID</label>
<input id="target_vmid" placeholder="%s">
</div>
<div class="field">
<label for="public_host_lxc" id="public-host-lxc-label">접속 주소 또는 도메인</label>
<input id="public_host_lxc" placeholder="1234.60.internal.kr">
<small id="public-host-lxc-help">설치 후 사용자가 접속할 예상 공개 주소입니다.</small>
</div>
<div class="field">
<label for="tls_mode_lxc" id="tls-mode-lxc-label">TLS 방식</label>
<select id="tls_mode_lxc">
<option value="later">나중에 설정</option>
<option value="existing">기존 인증서 사용</option>
</select>
<small id="tls-mode-lxc-help">LXC 설치 자체는 HTTP로 검증하고, 운영 전에 TLS를 정리할 수 있습니다.</small>
</div>
<div class="field full">
<label><input type="checkbox" id="host_companion"> <span id="host-companion-label">Proxmox host companion CLI도 함께 설치</span></label>
<small id="host-companion-help">선택하면 LXC 설치 후 Proxmox host에 proxmox-host-cli를 이어서 설치합니다.</small>
</div>
</div>
</div>
<div class="summary">
<strong id="summary-title">자동으로 결정되는 내부 설정</strong>
<ul>
<li id="summary-profile">설치 프로파일: Linux는 proxmox-host/proxmox-host-localvault, LXC는 proxmox-lxc-allinone</li>
<li id="summary-script">설치 스크립트: 서버 허용 목록에서 자동 사용</li>
<li id="summary-session">설치 단계: 방식 선택 → 입력 확인 → 검증/설치</li>
</ul>
</div>
<div class="companion-note">
<strong id="companion-title">Proxmox host companion</strong>
<span id="companion-copy">all-in-one LXC는 KeyCenter와 LocalVault 런타임만 담당합니다. Proxmox host의 boundary/CLI는 별도 proxmox-host-cli 단계로 설치합니다.</span>
</div>
<div class="actions">
<button class="btn btn-primary" id="save-quick">빠른 설치 저장</button>
<button class="btn btn-soft" id="validate-install">검증만 실행</button>
<button class="btn btn-primary" id="apply-install">설치 실행</button>
<button class="btn btn-soft" id="reload-state">상태 새로고침</button>
</div>
<div class="note" id="note-text">먼저 설치 방식을 고른 다음, 그 방식에 필요한 값만 입력하면 됩니다.</div>
<div class="summary" id="runtime-warning-box" style="display:none">
<strong id="runtime-warning-title">현재 호스트 경고</strong>
<ul id="runtime-warning-list"></ul>
</div>
<div class="status-box"><pre id="wizard-status">Loading install wizard state…</pre></div>
<div class="links">
<a href="/health">Health</a>
<a href="/ready">Ready</a>
<a href="/approve/install/bootstrap" id="bootstrap-link">Bootstrap Input</a>
<a href="/approve/install/custody" id="custody-link">Custody Input</a>
</div>
<details>
<summary id="advanced-summary">고급 설정 열기</summary>
<div class="advanced">
<div class="advanced-grid">
<div class="field">
<label for="flow">Flow</label>
<select id="flow"><option value="wizard">wizard</option><option value="quickstart">quickstart</option><option value="advanced">advanced</option></select>
</div>
<div class="field">
<label for="deployment_mode">Deployment Mode</label>
<input id="deployment_mode" placeholder="host-service">
</div>
<div class="field">
<label for="install_scope">Install Scope</label>
<input id="install_scope" placeholder="host-only">
</div>
<div class="field">
<label for="bootstrap_mode">Bootstrap Mode</label>
<input id="bootstrap_mode" placeholder="email">
</div>
<div class="field">
<label for="mail_transport">Mail Transport</label>
<input id="mail_transport" placeholder="smtp">
</div>
<div class="field">
<label for="install_profile">Install Profile</label>
<input id="install_profile" placeholder="proxmox-host">
</div>
<div class="field full">
<label for="install_script">Install Script</label>
<input id="install_script" placeholder="/opt/veilkey-selfhosted-repo/installer/install.sh">
</div>
<div class="field full">
<label for="install_workdir">Install Workdir</label>
<input id="install_workdir" placeholder="/opt/veilkey-selfhosted-repo/installer">
</div>
<div class="field full">
<label for="keycenter_url">KeyCenter URL</label>
<input id="keycenter_url" placeholder="https://keycenter.example.internal">
</div>
<div class="field">
<label for="tls_cert_path">TLS Cert Path</label>
<input id="tls_cert_path" placeholder="/etc/veilkey/tls/server.crt">
</div>
<div class="field">
<label for="tls_key_path">TLS Key Path</label>
<input id="tls_key_path" placeholder="/etc/veilkey/tls/server.key">
</div>
<div class="field full">
<label for="tls_ca_path">TLS CA Path</label>
<input id="tls_ca_path" placeholder="/etc/veilkey/tls/ca.crt">
</div>
<div class="field full">
<label for="planned_stages">Planned Stages</label>
<input id="planned_stages" placeholder="language,bootstrap,final_smoke">
</div>
</div>
<div class="actions">
<button class="btn btn-soft" id="save-session">세션만 저장</button>
<button class="btn btn-soft" id="save-runtime">런타임 설정만 저장</button>
</div>
</div>
</details>
</div>
</div>
<div class="card">
<div class="eyebrow" id="side-eyebrow">Why This Page Exists</div>
<h1 style="font-size:24px" id="side-title">리눅스 서버 설치를 먼저 끝냅니다</h1>
<p id="side-copy">브라우저는 설치 의도와 운영 정책만 저장합니다. 실제 설치 실행은 KeyCenter가 서버 측 runner를 통해 수행하며, 기본 추천 경로는 all-in-one LXC입니다.</p>
<div class="summary">
<strong id="steps-title">권장 순서</strong>
<ul>
<li id="step-1">1. 설치 대상을 고르고 접속 주소와 루트를 입력합니다.</li>
<li id="step-2">2. 먼저 검증만 실행으로 위험값과 프로파일을 확인합니다.</li>
<li id="step-3">3. 문제가 없으면 설치 실행을 누릅니다.</li>
<li id="step-4">4. 완료 후 /ready 가 열리면 운영 콘솔로 진입합니다.</li>
</ul>
</div>
<div class="summary">
<strong id="runs-title">최근 검증/설치 기록</strong>
<ul id="install-runs-list">
<li>아직 기록이 없습니다.</li>
</ul>
</div>
<div class="status-box"><pre id="wizard-preview"></pre></div>
</div>
</div>
<script>
const initialSessionState = %s;
const initialRuntimeConfig = %s;
const copy = {
  ko: {
    title: 'VeilKey 첫 설치 시작',
    subtitle: '먼저 설치 방식을 고르고, 다음 단계에서 그 방식에 맞는 최소 입력만 보여줍니다.',
    quickTitle: '어디에 설치할지 먼저 고릅니다',
    step1Copy: '여기서는 설치 방식만 정합니다. Proxmox 정보나 루트 경로는 다음 단계에서 필요한 경우에만 나옵니다.',
    step2Title: '선택한 방식에 맞는 정보만 입력합니다',
    step2Copy: 'Linux 설치는 도메인과 설치 루트만, Proxmox LXC 설치는 node와 VMID만 보입니다.',
    linuxOptionTitle: '일반 Linux 서버',
    linuxOptionCopy: '이 서버나 다른 Linux 서버에 직접 설치할 때 사용합니다. 기본 경로입니다.',
    lxcOptionTitle: 'Proxmox LXC 올인원',
    lxcOptionCopy: '새 LXC를 만들거나 기존 LXC에 all-in-one 런타임을 설치할 때만 선택합니다.',
    lxcModeLabel: 'LXC 방식',
    lxcModeHelp: 'Proxmox 대상 정보가 필요할 때만 이 단계가 열립니다.',
    targetNodeLabel: 'Proxmox node',
    targetVMIDLabel: 'LXC VMID',
    publicHostLabel: '접속 주소 또는 도메인',
    publicHostHelp: '설치 후 사용자가 접속할 예상 공개 주소입니다. 현재 wizard 주소는 자동 저장하지 않습니다.',
    publicHostLXCLabel: '접속 주소 또는 도메인',
    publicHostLXCHelp: '설치 후 사용자가 접속할 예상 공개 주소입니다.',
    tlsModeLabel: 'TLS 방식',
    tlsModeHelp: '처음 검증은 HTTP 기준으로 시작하고, 운영 전 TLS를 붙일 수 있습니다.',
    tlsModeLXCHelp: 'LXC 설치 자체는 HTTP로 검증하고, 운영 전에 TLS를 정리할 수 있습니다.',
    hostCompanionLabel: 'Proxmox host companion CLI도 함께 설치',
    hostCompanionHelp: '선택하면 LXC 설치 후 Proxmox host에 proxmox-host-cli를 이어서 설치합니다.',
    installRootLabel: '설치 대상 루트',
    installRootHelp: '리눅스 서버 직접 설치일 때만 사용합니다. 현재 서버의 live root면 추가 확인이 필요합니다.',
    localvaultLabel: '기존 LocalVault URL (선택)',
    localvaultHelp: '새 LocalVault를 같이 설치하지 않고, 기존 LocalVault를 연결할 때만 입력합니다.',
    summaryTitle: '자동으로 결정되는 내부 설정',
    summaryProfile: '설치 프로파일: Linux는 proxmox-host/proxmox-host-localvault, LXC는 proxmox-lxc-allinone',
    summaryScript: '설치 스크립트: 서버 허용 목록에서 자동 사용',
    summarySession: '설치 단계: 방식 선택 -> 입력 확인 -> 검증/설치',
    companionTitle: 'Proxmox host companion',
    companionCopy: 'all-in-one LXC는 KeyCenter와 LocalVault 런타임만 담당합니다. Proxmox host의 boundary/CLI는 별도 proxmox-host-cli 단계로 설치합니다.',
    confirmDangerousRoot: '이 서버의 루트(/)에 직접 설치하는 위험을 이해했고, 필요한 경우에만 실행합니다.',
    saveQuick: '빠른 설치 저장',
    validateInstall: '검증만 실행',
    applyInstall: '설치 실행',
    reload: '상태 새로고침',
    saveSession: '세션만 저장',
    saveRuntime: '런타임 설정만 저장',
    note: '먼저 설치 방식을 고른 다음, 그 방식에 필요한 값만 입력하면 됩니다.',
    advanced: '고급 설정 열기',
    sideEyebrow: 'First Install Guide',
    sideTitle: '설치 방식을 먼저 분리합니다',
    sideCopy: '브라우저는 설치 의도와 운영 정책만 저장합니다. Linux 서버 설치와 Proxmox LXC 설치를 같은 화면에서 섞지 않고, 선택한 방식에 따라 다음 단계만 보여줍니다.',
    stepsTitle: '권장 순서',
    step1: '1. 먼저 Linux 서버인지, Proxmox LXC인지 고릅니다.',
    step2: '2. 다음 단계에서 그 방식에 필요한 값만 입력합니다.',
    step3: '3. 먼저 검증만 실행으로 위험값과 해석된 프로파일을 확인합니다.',
    step4: '4. 문제가 없으면 설치 실행을 누릅니다.',
    bootstrap: 'Bootstrap 입력',
    custody: 'Custody 입력',
    loaded: '리눅스 설치 마법사 상태를 불러왔습니다.',
    running: '설치 실행이 진행 중입니다.',
    start: '설치 실행을 시작합니다...',
    saveOk: '빠른 설치 설정을 저장했습니다.',
    saveSessionOk: '설치 세션을 저장했습니다.',
    saveRuntimeOk: '런타임 설정을 저장했습니다.',
    loadFail: '설치 상태를 불러오지 못했습니다: ',
    saveFail: '빠른 설치 저장 실패: ',
    sessionFail: '세션 저장 실패: ',
    runtimeFail: '런타임 설정 저장 실패: ',
    applyFail: '설치 실행 실패: ',
    applyStarted: '설치 실행을 시작했습니다.',
    validateStarted: '설치 검증을 실행했습니다.',
    validateFail: '설치 검증 실패: ',
    runsTitle: '최근 검증/설치 기록',
    noRuns: '아직 기록이 없습니다.',
    runtimeWarningTitle: '현재 호스트 경고'
  },
  en: {
    title: 'Start the first VeilKey install',
    subtitle: 'Choose the install path first, then show only the minimum fields for that path.',
    quickTitle: 'Choose where to install first',
    step1Copy: 'This step only decides the install path. Proxmox metadata or root paths appear only when they are actually needed.',
    step2Title: 'Enter only the fields required for the selected path',
    step2Copy: 'Linux install shows domain and install root. Proxmox LXC install shows node and VMID.',
    linuxOptionTitle: 'General Linux server',
    linuxOptionCopy: 'Use this for a direct install onto this server or another Linux server. This is the default path.',
    lxcOptionTitle: 'Proxmox LXC all-in-one',
    lxcOptionCopy: 'Use this only when creating a new LXC or installing the all-in-one runtime into an existing LXC.',
    lxcModeLabel: 'LXC mode',
    lxcModeHelp: 'This section opens only when Proxmox target metadata is needed.',
    targetNodeLabel: 'Proxmox node',
    targetVMIDLabel: 'LXC VMID',
    publicHostLabel: 'Access host or domain',
    publicHostHelp: 'This is the expected public address after install. The wizard self URL is not auto-saved.',
    publicHostLXCLabel: 'Access host or domain',
    publicHostLXCHelp: 'This is the expected public address after install.',
    tlsModeLabel: 'TLS mode',
    tlsModeHelp: 'Start validation with HTTP, then attach TLS before production exposure.',
    tlsModeLXCHelp: 'Validate the LXC install with HTTP first, then finish TLS before production use.',
    hostCompanionLabel: 'Install Proxmox host companion CLI too',
    hostCompanionHelp: 'When enabled, the runner installs proxmox-host-cli on the Proxmox host after the LXC runtime finishes.',
    installRootLabel: 'Install root',
    installRootHelp: 'Use this only for a direct Linux host install. A live root install on the current host requires an extra confirmation.',
    localvaultLabel: 'Existing LocalVault URL (optional)',
    localvaultHelp: 'Fill this only when connecting an existing LocalVault instead of installing a new one.',
    summaryTitle: 'Derived internal settings',
    summaryProfile: 'Install profile: Linux uses proxmox-host/proxmox-host-localvault, LXC uses proxmox-lxc-allinone',
    summaryScript: 'Install script: auto-use server allowlisted runner',
    summarySession: 'Install stages: choose path -> confirm input -> validate/apply',
    companionTitle: 'Proxmox host companion',
    companionCopy: 'The all-in-one LXC only owns KeyCenter and LocalVault runtime. Install the Proxmox host boundary/CLI separately with proxmox-host-cli.',
    confirmDangerousRoot: 'I understand the risk of installing directly into the live root (/) and will only use it when intended.',
    saveQuick: 'Save Quick Setup',
    validateInstall: 'Validate Only',
    applyInstall: 'Apply Install',
    reload: 'Reload State',
    saveSession: 'Save Session Only',
    saveRuntime: 'Save Runtime Only',
    note: 'Choose the install path first, then fill only the fields for that path.',
    advanced: 'Open Advanced Settings',
    sideEyebrow: 'First Install Guide',
    sideTitle: 'Split the install paths first',
    sideCopy: 'The browser stores install intent and policy only. Keep Linux host install and Proxmox LXC install separate, and only reveal the next-step fields for the selected path.',
    stepsTitle: 'Recommended order',
    step1: '1. Choose Linux server or Proxmox LXC first.',
    step2: '2. Fill only the fields required for that path.',
    step3: '3. Validate first to confirm risky values and the resolved profile.',
    step4: '4. Apply install only after validation is clean.',
    bootstrap: 'Bootstrap Input',
    custody: 'Custody Input',
    loaded: 'Linux install wizard state loaded.',
    running: 'Install apply is running.',
    start: 'Starting install apply...',
    saveOk: 'Quick install settings saved.',
    saveSessionOk: 'Install session saved.',
    saveRuntimeOk: 'Runtime config saved.',
    loadFail: 'Failed to load install state: ',
    saveFail: 'Failed to save quick install: ',
    sessionFail: 'Failed to save install session: ',
    runtimeFail: 'Failed to save runtime config: ',
    applyFail: 'Failed to start install apply: ',
    applyStarted: 'Install apply started.',
    validateStarted: 'Install validation completed.',
    validateFail: 'Install validation failed: ',
    runsTitle: 'Recent validation/install runs',
    noRuns: 'No runs yet.',
    runtimeWarningTitle: 'Current host warning'
  }
};
const statusEl = document.getElementById('wizard-status');
const previewEl = document.getElementById('wizard-preview');
const runsEl = document.getElementById('install-runs-list');
const runtimeWarningBoxEl = document.getElementById('runtime-warning-box');
const runtimeWarningListEl = document.getElementById('runtime-warning-list');
const linuxFieldsEl = document.getElementById('linux-fields');
const lxcFieldsEl = document.getElementById('lxc-fields');
const quickFields = {
  target_mode: document.getElementById('target_mode'),
  lxc_mode: document.getElementById('lxc_mode'),
  target_node: document.getElementById('target_node'),
  target_vmid: document.getElementById('target_vmid'),
  public_host: document.getElementById('public_host'),
  tls_mode: document.getElementById('tls_mode'),
  public_host_lxc: document.getElementById('public_host_lxc'),
  tls_mode_lxc: document.getElementById('tls_mode_lxc'),
  host_companion: document.getElementById('host_companion')
};
const fields = {
  flow: document.getElementById('flow'),
  deployment_mode: document.getElementById('deployment_mode'),
  install_scope: document.getElementById('install_scope'),
  bootstrap_mode: document.getElementById('bootstrap_mode'),
  mail_transport: document.getElementById('mail_transport'),
  planned_stages: document.getElementById('planned_stages'),
  install_profile: document.getElementById('install_profile'),
  install_root: document.getElementById('install_root'),
  install_script: document.getElementById('install_script'),
  install_workdir: document.getElementById('install_workdir'),
  public_base_url: null,
  keycenter_url: document.getElementById('keycenter_url'),
  localvault_url: document.getElementById('localvault_url'),
  tls_cert_path: document.getElementById('tls_cert_path'),
  tls_key_path: document.getElementById('tls_key_path'),
  tls_ca_path: document.getElementById('tls_ca_path')
};
const confirmDangerousRootEl = document.getElementById('confirm-dangerous-root');
let currentLang = 'ko';

function currentPublicHostField() {
  return quickFields.target_mode.value === 'lxc-allinone' ? quickFields.public_host_lxc : quickFields.public_host;
}

function currentTLSModeField() {
  return quickFields.target_mode.value === 'lxc-allinone' ? quickFields.tls_mode_lxc : quickFields.tls_mode;
}

function deriveTargetModeFromProfile(profile) {
  switch ((profile || '').trim()) {
    case 'proxmox-lxc-allinone':
    case 'lxc-allinone':
    case 'all-in-one':
    case 'linux-all-in-one':
      return 'lxc-allinone';
    default:
      return 'linux-host';
  }
}

function guessCurrentHost() {
  const { protocol, hostname, port } = window.location;
  if (!hostname) {
    return '';
  }
  return protocol + '//' + hostname + (port ? ':' + port : '');
}

function deriveInstallProfile() {
  if (quickFields.target_mode.value === 'lxc-allinone') {
    return 'proxmox-lxc-allinone';
  }
  if (fields.localvault_url.value.trim()) {
    return 'proxmox-host-localvault';
  }
  return 'proxmox-host';
}

function deriveInstallScript(existingValue) {
  return existingValue || '/opt/veilkey-selfhosted-repo/installer/install.sh';
}

function deriveInstallWorkdir(existingValue) {
  return existingValue || '/opt/veilkey-selfhosted-repo/installer';
}

function syncDerivedFields() {
  const hostField = currentPublicHostField();
  const tlsField = currentTLSModeField();
  const rawHost = hostField.value.trim();
  const guessed = guessCurrentHost();
  const hasScheme = rawHost.startsWith('http://') || rawHost.startsWith('https://');
  const tlsLater = tlsField.value === 'later';
  let baseURL = rawHost;
  if (baseURL && !hasScheme) {
    baseURL = (tlsLater ? 'http://' : 'https://') + baseURL;
  }
  if (!baseURL) {
    baseURL = quickFields.target_mode.value === 'linux-host' ? guessed : '';
  }
  fields.install_profile.value = deriveInstallProfile();
  fields.install_script.value = deriveInstallScript(fields.install_script.value.trim());
  fields.install_workdir.value = deriveInstallWorkdir(fields.install_workdir.value.trim());
  if (quickFields.target_mode.value === 'lxc-allinone') {
    fields.install_root.value = '/';
    fields.keycenter_url.value = '';
    quickFields.public_host.value = '';
  } else {
    fields.keycenter_url.value = baseURL;
    quickFields.public_host_lxc.value = '';
  }
  fields.deployment_mode.value = quickFields.target_mode.value === 'lxc-allinone' ? 'lxc-allinone' : 'host-service';
  fields.install_scope.value = quickFields.target_mode.value === 'lxc-allinone' ? 'all-in-one' : (fields.localvault_url.value.trim() ? 'host+existing-localvault' : 'host-only');
  fields.bootstrap_mode.value = 'email';
  fields.mail_transport.value = 'smtp';
  fields.flow.value = 'wizard';
  fields.planned_stages.value = 'language,bootstrap,final_smoke';
  if (tlsLater) {
    fields.tls_cert_path.value = '';
    fields.tls_key_path.value = '';
    fields.tls_ca_path.value = '';
  }
}

function renderRuntimeWarning(message) {
  if (!message) {
    runtimeWarningBoxEl.style.display = 'none';
    runtimeWarningListEl.innerHTML = '';
    return;
  }
  runtimeWarningBoxEl.style.display = '';
  runtimeWarningListEl.innerHTML = '<li>' + message + '</li>';
}

function updateTargetSpecificUI() {
  const isLXC = quickFields.target_mode.value === 'lxc-allinone';
  document.querySelectorAll('input[name="target_mode_choice"]').forEach((el) => {
    el.checked = el.value === quickFields.target_mode.value;
  });
  linuxFieldsEl.classList.toggle('hidden', isLXC);
  lxcFieldsEl.classList.toggle('hidden', !isLXC);
  document.getElementById('option-linux-host').classList.toggle('active', !isLXC);
  document.getElementById('option-lxc-allinone').classList.toggle('active', isLXC);
  document.getElementById('install_root').disabled = isLXC;
  confirmDangerousRootEl.checked = isLXC ? false : confirmDangerousRootEl.checked;
  confirmDangerousRootEl.disabled = isLXC;
  quickFields.host_companion.disabled = !isLXC;
  if (!isLXC) {
    quickFields.host_companion.checked = false;
  }
}

function setStatus(message) {
  statusEl.textContent = message;
}

function renderPreview() {
  syncDerivedFields();
  document.getElementById('target-chip').textContent = 'target: ' + quickFields.target_mode.value;
  const preview = {
    language: currentLang,
    target: quickFields.target_mode.value,
    session: {
      flow: fields.flow.value,
      deployment_mode: fields.deployment_mode.value,
      install_scope: fields.install_scope.value,
      bootstrap_mode: fields.bootstrap_mode.value,
      mail_transport: fields.mail_transport.value,
      planned_stages: fields.planned_stages.value.split(',').map((item) => item.trim()).filter(Boolean)
    },
    quick_setup: {
      target_mode: quickFields.target_mode.value,
      lxc_mode: quickFields.lxc_mode.value,
      target_node: quickFields.target_node.value,
      target_vmid: quickFields.target_vmid.value,
      host_companion: quickFields.host_companion.checked,
      public_host: currentPublicHostField().value,
      tls_mode: currentTLSModeField().value,
      install_root: fields.install_root.value,
      localvault_url: fields.localvault_url.value
    },
    runtime_config: {
      target_type: quickFields.target_mode.value,
      target_mode: quickFields.lxc_mode.value,
      target_node: quickFields.target_node.value,
      target_vmid: quickFields.target_vmid.value,
      host_companion: quickFields.host_companion.checked,
      public_base_url: currentPublicHostField().value,
      install_profile: fields.install_profile.value,
      install_root: fields.install_root.value,
      install_script: fields.install_script.value,
      install_workdir: fields.install_workdir.value,
      keycenter_url: fields.keycenter_url.value,
      localvault_url: fields.localvault_url.value,
      tls_cert_path: fields.tls_cert_path.value,
      tls_key_path: fields.tls_key_path.value,
      tls_ca_path: fields.tls_ca_path.value
    },
    apply: window.installApplyState || null,
    validation: window.installValidationState || null
  };
  previewEl.textContent = JSON.stringify(preview, null, 2);
  updateTargetSpecificUI();
}

function renderRuns(runs) {
  const items = Array.isArray(runs) ? runs : [];
  if (!items.length) {
    runsEl.innerHTML = '<li>' + copy[currentLang].noRuns + '</li>';
    return;
  }
  runsEl.innerHTML = items.slice(0, 5).map((run) => {
    const summary = [
      run.run_kind,
      run.status,
      run.install_profile,
      run.install_root
    ].filter(Boolean).join(' | ');
    const extra = run.last_error ? ' - ' + run.last_error : '';
    return '<li><strong>' + summary + '</strong>' + extra + '</li>';
  }).join('');
}

function setLanguage(lang) {
  currentLang = lang;
  document.getElementById('lang-ko').classList.toggle('active', lang === 'ko');
  document.getElementById('lang-en').classList.toggle('active', lang === 'en');
  const t = copy[lang];
  document.getElementById('title').textContent = t.title;
  document.getElementById('subtitle').textContent = t.subtitle;
  document.getElementById('quick-title').textContent = t.quickTitle;
  document.getElementById('step-1-copy').textContent = t.step1Copy;
  document.getElementById('step-2-title').textContent = t.step2Title;
  document.getElementById('step-2-copy').textContent = t.step2Copy;
  document.getElementById('linux-option-title').textContent = t.linuxOptionTitle;
  document.getElementById('linux-option-copy').textContent = t.linuxOptionCopy;
  document.getElementById('lxc-option-title').textContent = t.lxcOptionTitle;
  document.getElementById('lxc-option-copy').textContent = t.lxcOptionCopy;
  document.getElementById('lxc-mode-label').textContent = t.lxcModeLabel;
  document.getElementById('lxc-mode-help').textContent = t.lxcModeHelp;
  document.getElementById('target-node-label').textContent = t.targetNodeLabel;
  document.getElementById('target-vmid-label').textContent = t.targetVMIDLabel;
  document.getElementById('public-host-label').textContent = t.publicHostLabel;
  document.getElementById('public-host-help').textContent = t.publicHostHelp;
  document.getElementById('public-host-lxc-label').textContent = t.publicHostLXCLabel;
  document.getElementById('public-host-lxc-help').textContent = t.publicHostLXCHelp;
  document.getElementById('tls-mode-label').textContent = t.tlsModeLabel;
  document.getElementById('tls-mode-help').textContent = t.tlsModeHelp;
  document.getElementById('tls-mode-lxc-label').textContent = t.tlsModeLabel;
  document.getElementById('tls-mode-lxc-help').textContent = t.tlsModeLXCHelp;
  document.getElementById('host-companion-label').textContent = t.hostCompanionLabel;
  document.getElementById('host-companion-help').textContent = t.hostCompanionHelp;
  document.getElementById('install-root-label').textContent = t.installRootLabel;
  document.getElementById('install-root-help').textContent = t.installRootHelp;
  document.getElementById('localvault-label').textContent = t.localvaultLabel;
  document.getElementById('localvault-help').textContent = t.localvaultHelp;
  document.getElementById('summary-title').textContent = t.summaryTitle;
  document.getElementById('summary-profile').textContent = t.summaryProfile;
  document.getElementById('summary-script').textContent = t.summaryScript;
  document.getElementById('summary-session').textContent = t.summarySession;
  document.getElementById('companion-title').textContent = t.companionTitle;
  document.getElementById('companion-copy').textContent = t.companionCopy;
  document.getElementById('confirm-dangerous-root-label').textContent = t.confirmDangerousRoot;
  document.getElementById('save-quick').textContent = t.saveQuick;
  document.getElementById('validate-install').textContent = t.validateInstall;
  document.getElementById('apply-install').textContent = t.applyInstall;
  document.getElementById('reload-state').textContent = t.reload;
  document.getElementById('save-session').textContent = t.saveSession;
  document.getElementById('save-runtime').textContent = t.saveRuntime;
  document.getElementById('note-text').textContent = t.note;
  document.getElementById('advanced-summary').textContent = t.advanced;
  document.getElementById('side-eyebrow').textContent = t.sideEyebrow;
  document.getElementById('side-title').textContent = t.sideTitle;
  document.getElementById('side-copy').textContent = t.sideCopy;
  document.getElementById('steps-title').textContent = t.stepsTitle;
  document.getElementById('step-1').textContent = t.step1;
  document.getElementById('step-2').textContent = t.step2;
  document.getElementById('step-3').textContent = t.step3;
  document.getElementById('step-4').textContent = t.step4;
  document.getElementById('runs-title').textContent = t.runsTitle;
  document.getElementById('runtime-warning-title').textContent = t.runtimeWarningTitle;
  document.getElementById('bootstrap-link').textContent = t.bootstrap;
  document.getElementById('custody-link').textContent = t.custody;
  renderRuns(window.installRuns || []);
  updateTargetSpecificUI();
}

function applySessionState(data) {
  const session = data && data.exists ? data.session : null;
  fields.flow.value = (session && session.flow) || 'wizard';
  fields.deployment_mode.value = (session && session.deployment_mode) || 'host-service';
  fields.install_scope.value = (session && session.install_scope) || 'host-only';
  fields.bootstrap_mode.value = (session && session.bootstrap_mode) || 'email';
  fields.mail_transport.value = (session && session.mail_transport) || 'smtp';
  fields.planned_stages.value = session && Array.isArray(session.planned_stages) ? session.planned_stages.join(',') : 'language,bootstrap,final_smoke';
}

function applyRuntimeConfig(data) {
  const publicBaseURL = data.public_base_url || '';
  const hasHTTPSURL = publicBaseURL.startsWith('https://');
  const hasTLSArtifacts = !!(data.tls_cert_path || data.tls_key_path);
  fields.install_profile.value = data.install_profile || 'proxmox-host';
  quickFields.target_mode.value = data.target_type || deriveTargetModeFromProfile(fields.install_profile.value);
  quickFields.lxc_mode.value = data.target_mode || 'new';
  quickFields.target_node.value = data.target_node || '';
  quickFields.target_vmid.value = data.target_vmid || '';
  quickFields.host_companion.checked = !!data.host_companion;
  fields.install_root.value = data.install_root || '/';
  fields.install_script.value = data.install_script || '';
  fields.install_workdir.value = data.install_workdir || '';
  fields.keycenter_url.value = data.keycenter_url || '';
  quickFields.public_host.value = '';
  quickFields.public_host_lxc.value = '';
  currentPublicHostField().value = publicBaseURL.replace(/^https?:\/\//, '');
  fields.localvault_url.value = data.localvault_url || '';
  fields.tls_cert_path.value = data.tls_cert_path || '';
  fields.tls_key_path.value = data.tls_key_path || '';
  fields.tls_ca_path.value = data.tls_ca_path || '';
  quickFields.tls_mode.value = (hasTLSArtifacts || hasHTTPSURL) ? 'existing' : 'later';
  quickFields.tls_mode_lxc.value = quickFields.tls_mode.value;
  document.getElementById('target-chip').textContent = 'target: ' + quickFields.target_mode.value;
  renderRuntimeWarning(data.runtime_warning || '');
  updateTargetSpecificUI();
}

async function request(path, options) {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...(options || {})
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || ('HTTP ' + response.status));
  }
  return body;
}

async function reloadState() {
  try {
    const [sessionResp, runtimeResp, applyResp, runsResp] = await Promise.all([
      request('/api/install/state'),
      request('/api/install/runtime-config'),
      request('/api/install/apply'),
      request('/api/install/runs')
    ]);
    applySessionState(sessionResp);
    applyRuntimeConfig(runtimeResp);
    window.installApplyState = applyResp;
    window.installRuns = runsResp.runs || [];
    renderRuns(window.installRuns);
    renderPreview();
    setStatus(applyResp.install_running ? copy[currentLang].running : copy[currentLang].loaded);
  } catch (error) {
    setStatus(copy[currentLang].loadFail + error.message);
  }
}

async function saveSession() {
  try {
    syncDerivedFields();
    const existing = await request('/api/install/state');
    const payload = {
      session_id: existing.exists && existing.session ? existing.session.session_id : '',
      version: existing.exists && existing.session ? existing.session.version : 1,
      language: currentLang,
      quickstart: existing.exists && existing.session ? !!existing.session.quickstart : false,
      flow: fields.flow.value || 'wizard',
      deployment_mode: fields.deployment_mode.value || 'host-service',
      install_scope: fields.install_scope.value || 'host-only',
      bootstrap_mode: fields.bootstrap_mode.value || 'email',
      mail_transport: fields.mail_transport.value || 'smtp',
      planned_stages: fields.planned_stages.value.split(',').map((item) => item.trim()).filter(Boolean),
      completed_stages: existing.exists && existing.session ? (existing.session.completed_stages || []) : [],
      last_stage: existing.exists && existing.session && existing.session.last_stage ? existing.session.last_stage : 'language'
    };
    await request('/api/install/session', { method: 'POST', body: JSON.stringify(payload) });
    await reloadState();
    setStatus(copy[currentLang].saveSessionOk);
  } catch (error) {
    setStatus(copy[currentLang].sessionFail + error.message);
  }
}

async function saveRuntimeConfig() {
  try {
    syncDerivedFields();
    const payload = {
      target_type: quickFields.target_mode.value,
      target_mode: quickFields.lxc_mode.value,
      target_node: quickFields.target_node.value,
      target_vmid: quickFields.target_vmid.value,
      host_companion: quickFields.host_companion.checked,
      public_base_url: currentPublicHostField().value ? ((currentPublicHostField().value.startsWith('http://') || currentPublicHostField().value.startsWith('https://')) ? currentPublicHostField().value : ((currentTLSModeField().value === 'later' ? 'http://' : 'https://') + currentPublicHostField().value)) : '',
      install_profile: fields.install_profile.value,
      install_root: fields.install_root.value,
      install_script: fields.install_script.value,
      install_workdir: fields.install_workdir.value,
      keycenter_url: fields.keycenter_url.value,
      localvault_url: fields.localvault_url.value,
      tls_cert_path: fields.tls_cert_path.value,
      tls_key_path: fields.tls_key_path.value,
      tls_ca_path: fields.tls_ca_path.value
    };
    await request('/api/install/runtime-config', { method: 'PATCH', body: JSON.stringify(payload) });
    renderPreview();
    setStatus(copy[currentLang].saveRuntimeOk);
  } catch (error) {
    setStatus(copy[currentLang].runtimeFail + error.message);
  }
}

async function saveQuickSetup() {
  try {
    await saveSession();
    await saveRuntimeConfig();
    setStatus(copy[currentLang].saveOk);
  } catch (error) {
    setStatus(copy[currentLang].saveFail + error.message);
  }
}

async function validateInstall() {
  try {
    syncDerivedFields();
    await saveSession();
    await saveRuntimeConfig();
    const response = await fetch('/api/install/validate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        confirm_dangerous_root: !!confirmDangerousRootEl.checked
      })
    });
    const body = await response.json().catch(() => ({}));
    window.installValidationState = body.validation || null;
    await reloadState();
    if (!response.ok) {
      throw new Error((body.validation && body.validation.errors && body.validation.errors.join(', ')) || body.error || ('HTTP ' + response.status));
    }
    setStatus(copy[currentLang].validateStarted);
  } catch (error) {
    setStatus(copy[currentLang].validateFail + error.message);
  }
}

async function applyInstall() {
  try {
    syncDerivedFields();
    setStatus(copy[currentLang].start);
    const resp = await request('/api/install/apply', {
      method: 'POST',
      body: JSON.stringify({
        confirm_dangerous_root: !!confirmDangerousRootEl.checked
      })
    });
    window.installApplyState = resp;
    renderPreview();
    setStatus(copy[currentLang].applyStarted);
    setTimeout(reloadState, 500);
  } catch (error) {
    setStatus(copy[currentLang].applyFail + error.message);
  }
}

document.getElementById('lang-ko').addEventListener('click', () => { setLanguage('ko'); renderPreview(); });
document.getElementById('lang-en').addEventListener('click', () => { setLanguage('en'); renderPreview(); });
document.getElementById('save-quick').addEventListener('click', saveQuickSetup);
document.getElementById('validate-install').addEventListener('click', validateInstall);
document.getElementById('save-session').addEventListener('click', saveSession);
document.getElementById('save-runtime').addEventListener('click', saveRuntimeConfig);
document.getElementById('apply-install').addEventListener('click', applyInstall);
document.getElementById('reload-state').addEventListener('click', reloadState);
document.querySelectorAll('input[name="target_mode_choice"]').forEach((el) => el.addEventListener('change', (event) => {
  quickFields.target_mode.value = event.target.value;
  renderPreview();
}));
Object.values(fields).forEach((el) => el.addEventListener('input', renderPreview));
Object.values(quickFields).forEach((el) => el.addEventListener('input', renderPreview));

setLanguage((initialSessionState.session && initialSessionState.session.language) || 'ko');
applySessionState(initialSessionState);
applyRuntimeConfig(initialRuntimeConfig);
renderPreview();
reloadState();
</script></body></html>`
