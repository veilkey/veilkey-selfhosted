# VeilKey Self-Hosted

영문 문서: [`README.md`](./README.md)

**AI가 절대 볼 수 없는 시크릿 관리. PTY 레벨 양방향 마스킹 + 블록체인 감사.**

## 핵심 기능

- **PTY 양방향 마스킹** — 터미널 출력에서 비밀번호가 나오면 자동으로 `VK:LOCAL:xxx`로 치환. AI 코딩 도구가 절대 평문을 볼 수 없음.
- **분리 보관** — VaultCenter(열쇠) + LocalVault(금고). 한쪽만 탈취해도 복호화 불가.
- **블록체인 감사** — CometBFT로 모든 키 운영 이력 불변 기록.

## 구조

```
VaultCenter (중앙 관리)           LocalVault (로컬 금고)
┌───────────────────┐          ┌──────────────────┐
│ agentDEK 보관      │          │ ciphertext 저장    │
│ 암호화/복호화 수행   │          │ 복호화 불가        │
│ CometBFT 감사 체인  │          │ heartbeat 전송     │
│ 관리자 웹 UI       │          │                    │
└───────────────────┘          └──────────────────┘

veil CLI (PTY 마스킹)
┌──────────────────────────────────────────┐
│ 환경변수: VK:LOCAL:xxx → 실제 값 (프로세스) │
│ 출력: 실제 값 → VK:LOCAL:xxx (화면/AI)     │
└──────────────────────────────────────────┘
```

## 빠른 시작

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
docker compose up -d
```

1. `https://localhost:11181` → 마스터 + 관리자 비밀번호 설정
2. 키센터에서 등록 토큰 발급 → LocalVault init
3. 키센터에서 임시키 생성 → 볼트에 격상
4. veil 셸 진입: `docker compose exec -it veil veilkey-cli wrap-pty bash`

## CLI 도구

| 명령 | 용도 |
|------|------|
| `veilkey-cli resolve VK:LOCAL:xxx` | ref → 실제 값 |
| `veilkey-cli exec echo VK:LOCAL:xxx` | 인자 치환 후 실행 |
| `veilkey-cli wrap-pty bash` | PTY 마스킹 셸 진입 |
| `veilkey-cli scan file.env` | 시크릿 감지 (222 패턴) |
| `veilkey-cli status` | 연결 상태 확인 |

## 라이선스

GNU Affero General Public License v3.0 (AGPL-3.0). [`LICENSE`](./LICENSE) 참조.

## 보안 고지

VeilKey는 시크릿과 암호화 자료를 다루는 보안 민감 도구입니다.
프로덕션 사용 전 자체 보안 감사를 수행하세요.
보안 이슈 발견 시 GitHub Security Advisories로 비공개 보고해주세요.
