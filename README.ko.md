# VeilKey Self-Hosted

영문 문서: [`README.md`](./README.md)

`veilkey-selfhosted`는 VeilKey의 self-hosted 제품 레포입니다.

공식 도메인: `https://veil-key.com`

이 레포에는 직접 인프라에 설치해서 돌리기 위한 핵심 요소가 같이 들어 있습니다.

- `VaultCenter`
  - 중앙 제어면
- `LocalVault`
  - 노드/컨테이너 근처에서 도는 로컬 런타임
- `CLI`
  - 운영자 진입점
- `Proxy`
  - 외부 실행 경계
- `installer`
  - 설치 및 검증 계층

운영자 진입점은 현재 이렇게 나뉩니다.

- `veil`
  - 보호된 세션 진입점
- `veilkey`
  - 그 세션 안팎에서 상태, 암복호화, 정책을 다루는 명령
- `veilkey-cli`
  - 래퍼와 설치 스크립트가 사용하는 하위 구현 바이너리

## 핵심 로직

가장 짧게 이해하면 이렇습니다.

1. `VaultCenter`가 중앙에서 정책과 카탈로그를 관리합니다.
2. 여러 개의 `LocalVault`가 각각의 호스트나 컨테이너 안에서 돌아갑니다.
3. 운영자는 CLI와 설치 흐름으로 이 노드들을 등록하고 확인하고 업데이트합니다.
4. 실제 런타임 변경은 heartbeat, tracked-ref sync, bulk-apply 흐름을 통해 퍼집니다.

구조는 보통 이렇게 봐도 됩니다.

```text
운영자 / CLI
     |
     v
 VaultCenter
     |
     +---- LocalVault (컨테이너 A)
     +---- LocalVault (컨테이너 B)
     +---- LocalVault (호스트 노드)
```

즉:

- `VaultCenter`는 중앙에서 관리한다
- `LocalVault`는 각 노드에서 실제로 동작한다

## 왜 self-hosted 인가

VeilKey의 핵심 가치는 다음을 직접 통제하는 데 있습니다.

- ciphertext와 런타임 상태가 어디에 저장되는지
- 노드 identity와 정책이 어떻게 강제되는지
- 컨테이너가 어떻게 프로비저닝되고 관리되는지
- 키 변경과 재바인딩이 어떻게 적용되는지

그래서 이 레포는 hosted SaaS 비밀관리 서비스보다는, 직접 설치해서 운영하는 런타임 제품에 가깝습니다.

## 중앙 관리와 일괄 변경

VeilKey는 “각 노드가 제각각 알아서 관리하는 구조”가 아닙니다.

핵심은:

- `LocalVault`는 `VaultCenter`에 등록됩니다
- 중앙에서 각 노드의 상태와 binding을 볼 수 있습니다
- 여러 노드에 대해 bulk-apply 형태의 변경을 밀 수 있습니다
- 회전(rotation)이나 재바인딩(rebind)도 중앙 모델 안에서 다룹니다

즉 “중앙에서 관리할 수 있고, 여러 노드에 일괄 적용할 수 있다”는 점이 중요한 축입니다.

## key version 과 버전 관리

여기서 중요한 값은 몇 개가 있습니다.

- `key_version`
  - 현재 노드가 반영한 키/런타임 버전
- `vault_hash`
  - 안정적인 vault 식별자
- `vault_runtime_hash`
  - 현재 VaultCenter binding 기준 해시
- `managed_paths`
  - 이 노드가 다루는 경로 정보

흐름은 대략 이렇습니다.

1. LocalVault가 VaultCenter에 heartbeat를 보냅니다
2. VaultCenter가 rotation 또는 rebind를 요구할 수 있습니다
3. LocalVault가 새 `key_version`을 반영합니다
4. 다시 heartbeat 하면서 최신 runtime binding 상태를 보고합니다

즉:

- 버전은 중앙에서 관리되고
- 반영은 각 LocalVault에서 실행되고
- 상태는 다시 중앙으로 올라옵니다

## 빠른 시작

가장 빠른 시작점은 installer입니다.

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted/installer
./install.sh validate
```

그 다음 실제 설치는 여기서 봅니다.

- [`installer/INSTALL.md`](./installer/INSTALL.md)

최소 성공 확인은 이 정도까지 보이면 됩니다.

```bash
curl http://127.0.0.1:10181/health
curl http://127.0.0.1:10180/health
```

기대 결과:

- VaultCenter health 응답
- LocalVault health 응답
- 등록 후 중앙 화면이나 상태 조회에서 노드가 보임

### 성공했을 때 보이는 것

VaultCenter는 처음엔 locked 상태로 뜨고, unlock 뒤에 정상 상태로 바뀝니다.

```json
GET /health
{"status":"locked"}
```

```json
POST /api/unlock
{"status":"unlocked"}
```

```json
GET /health
{"status":"ok"}
```

LocalVault 쪽에서는 운영 흐름에서 이런 출력이 보입니다.

```text
heartbeat sent
rotation applied and heartbeat sent
rebind prepared with key_version=9
```

## Veil 세션 사용 모델

의도한 CLI 사용 흐름은 이렇습니다.

```bash
veil
claude
codex
```

관리 명령은 `veilkey`로 처리합니다.

```bash
veilkey status
veilkey paste-mode on
veilkey resolve VK:LOCAL:example
```

즉 `veil`은 보호된 환경에 들어가는 명령이고, `veilkey`는 그 안에서 정책과 키 동작을 다루는 명령입니다.

## 이게 아닌 것

- hosted SaaS 비밀관리 서비스
- 개인용 범용 비밀번호 저장소
- 단일 바이너리 로컬 secret 도구
- 노드 런타임과 분리된 cloud-only 제어면

## 트레이드오프

- hosted 서비스보다 운영 복잡도가 높습니다
- 호스트, 네트워크 설정 영향을 더 많이 받습니다
- 설치와 런타임 검증을 직접 해야 하는 대신, 제어권도 직접 가집니다

## 대략적인 비교

| 도구 형태 | 기본 모델 | VeilKey 차이점 |
|---|---|---|
| hosted secret SaaS | 중앙 hosted control plane | 런타임과 상태를 직접 인프라 안에 둡니다 |
| 범용 비밀번호 관리자 | 저장/조회 중심 | 노드 등록, runtime identity, 정책 기반 실행까지 다룹니다 |
| 파일 암호화 워크플로우 | 저장소 파일 암호화 | VaultCenter + 여러 LocalVault + heartbeat/rebind 흐름이 있습니다 |

## 기여

기여 규칙은 여기서 시작합니다.

- [`CONTRIBUTING.md`](./CONTRIBUTING.md)

## 라이선스

이 저장소는 GNU Affero General Public License v3.0 (AGPL-3.0)으로 배포됩니다.

[`LICENSE`](./LICENSE)를 참조하세요.

## 보안 고지

VeilKey는 시크릿과 암호화 자료를 다루는 보안 민감 도구입니다.
이 소프트웨어는 어떠한 보증도 없이 제공됩니다. 프로덕션 사용 전에
자체 보안 감사를 수행하세요. 보안 이슈를 발견하면 공개 이슈 대신
GitHub Security Advisories를 통해 비공개로 보고해주세요.
