# wingsv-panel

Веб-панель и публичный лендинг для WINGS V.

## Назначение

Один Go-бинарник плюс Vue 3 SPA. Сервис закрывает три задачи:

- **Лендинг.** Ссылки `wingsv://` и `vless://` разбираются на читаемое превью. Кнопка ниже инициирует открытие WINGS V; при отсутствии установленного приложения предлагается скачивание последнего APK с GitHub Releases. APK сохраняется в кеш браузера для повторных визитов без сети.
- **Админ-панель.** Аутентифицированные администраторы создают клиентов и редактируют их конфигурации (форма либо JSON). Сидинг нового клиента поддерживается из существующего клиента, из ссылки `wingsv://` либо `vless://`. Master-config обеспечивает массовое применение общих настроек.
- **Guardian.** Долгоживущий WebSocket-канал между сервером и устройствами WINGS V. Через канал администратору доступны телеметрия клиента, потоки логов, push новой конфигурации и команды (старт/стоп туннеля, обновление подписок и др.). Учётная запись owner имеет доступ ко всем администраторам и всем клиентам.

## Запуск из бинарника

GitHub Actions при пуше тега `v*` собирает бинарники под Linux: amd64, arm, arm64, riscv64. Фронтенд встроен в бинарник через `go:embed` — отдельная директория `web/dist/` рядом не требуется. Загрузка из релиза:

```bash
curl -L -o wingsv-panel \
  https://github.com/WINGS-N/wingsv-panel/releases/latest/download/wingsv-panel-linux-amd64
chmod +x wingsv-panel
./wingsv-panel
```

Минимальный набор переменных окружения:

```bash
LISTEN_ADDR=":8080" \
PUBLIC_BASE_URL="https://panel.example.com" \
DB_PATH="./wingsv.db" \
BOOTSTRAP_ADMIN_USERNAME="admin" \
BOOTSTRAP_ADMIN_PASSWORD="<новый-пароль>" \
./wingsv-panel
```

При необходимости подменить фронтенд без пересборки бинарника достаточно указать `STATIC_DIR` с путём к собственной директории `web/dist/`.

## Запуск из исходников

```bash
./scripts/generate_proto.sh
go mod tidy
pnpm install --frozen-lockfile
pnpm build
go run ./cmd/server
```

Сервис будет доступен на `http://localhost:8080`. Учётная запись по умолчанию — `admin` / `admin`; её рекомендуется заменить переменными окружения `BOOTSTRAP_ADMIN_USERNAME` и `BOOTSTRAP_ADMIN_PASSWORD` до первого запуска.

## Установка одной командой (install.sh)

Готовый инсталлятор для отдельного сервера. По умолчанию ставит панель бинарём
(флаг `--docker` — контейнером), спрашивает про порт, сертификат (Let's Encrypt
через acme.sh / готовый cert / self-signed со своим CA — с доменом или по IP),
бутстрап-админа, а также
предлагает поднять локальную ноду vk-turn-proxy и (если найден) подключить
локальный 3x-ui для создания профилей.

```bash
curl -fsSL https://raw.githubusercontent.com/WINGS-N/wingsv-panel/main/install.sh | sudo bash
# опции: --docker  --yes  --uninstall
```

Панель отдаёт HTTPS сама (env `TLS_CERT`/`TLS_KEY`, либо `TLS_SELF_SIGNED=true` —
leaf из встроенного CA; SPKI-пин попадает в enrollment-ссылки приложения).
Скрипт кладёт конфиги в `/etc/wings/`, данные в `/var/lib/wings/panel`, ставит
systemd-юниты `wingsv-panel` и `wings-vktp`.

## Запуск через Docker

```bash
docker compose up --build
```

Публичный образ:

```
ghcr.io/wings-n/wingsv-panel:latest
```

Обновление образа на промышленной среде:

```bash
docker build -t ghcr.io/wings-n/wingsv-panel:latest .
docker push ghcr.io/wings-n/wingsv-panel:latest
```

## Запуск в Kubernetes

Манифесты удалены: развёртывание переезжает на Helm-чарт. До его появления
последнюю рабочую версию манифестов можно достать из истории git:

```bash
git show HEAD~1:k8s/ >/dev/null   # список файлов
git checkout HEAD~1 -- k8s/       # вернуть в рабочее дерево
```

Обратите внимание: Guardian gRPC и provisioning gRPC ходят на тот же публичный
хост, что и веб, но маршрутизируются по префиксу метода на порт 9091 (h2c) -
чарт обязан воспроизвести оба маршрута, иначе устройства не подключатся.

Обновление развёрнутого приложения:

```bash
docker build -t ghcr.io/wings-n/wingsv-panel:latest .
docker push ghcr.io/wings-n/wingsv-panel:latest
kubectl -n v-wingsnet rollout restart deploy/app
```

## Proto

`wingsv.proto` подключён через subtree:

```bash
git subtree pull --prefix=external/wingsv-proto <wingsv-proto-repo> main --squash
./scripts/generate_proto.sh
```

## Assetlinks (Android App Link)

Пример находится в `web/public/.well-known/assetlinks.json.example`. В промышленной среде необходимо либо разместить реальный `assetlinks.json` по тому же пути, либо задать переменную окружения `ASSET_LINKS_JSON` с готовым содержимым — её значение имеет приоритет.

Если ни переменная окружения, ни файл не заданы, эндпоинт `/.well-known/assetlinks.json` возвращает HTTP 404 с телом `{"error":true,"message":"asset links not configured"}`. Лендинг и скачивание APK продолжат работать, однако Android перестаёт считать домен верифицированным владельцем WINGS V: при переходе по ссылке `wingsv://` система покажет диалог выбора приложения вместо прямого запуска.
