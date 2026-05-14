<template>
  <div class="config-form">
    <!-- App preferences -->
    <section v-if="show('app')" class="form-section">
      <h3 class="form-section-title">Приложение</h3>
      <div class="form-row">
        <label class="form-label">Тема</label>
        <OneuiSelect :model-value="ap.themeMode || 'THEME_MODE_UNSPECIFIED'" :options="themeOptions" @change="setAp('themeMode', $event === 'THEME_MODE_UNSPECIFIED' ? undefined : $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">DNS resolver</label>
        <OneuiSelect :model-value="ap.dnsMode || 'DNS_MODE_UNSPECIFIED'" :options="dnsOptions" @change="setAp('dnsMode', $event === 'DNS_MODE_UNSPECIFIED' ? undefined : $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Автозапуск при загрузке</label>
        <OneuiSwitch :model-value="!!ap.autoStartOnBoot" @change="setAp('autoStartOnBoot', $event)" />
      </div>
    </section>

    <!-- VK TURN -->
    <section v-if="show('vk_turn')" class="form-section">
      <h3 class="form-section-title">VK TURN</h3>
      <div class="form-row">
        <label class="form-label">Под-backend</label>
        <OneuiSelect :model-value="turn.tunnelMode || 'TUNNEL_MODE_WIREGUARD'" :options="tunnelModeOptions" @change="setTurn('tunnelMode', $event === 'TUNNEL_MODE_WIREGUARD' ? undefined : $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Endpoint</label>
        <input class="text-input" :value="turn.endpoint?.host || ''" @input="setTurnHost($event.target.value)" placeholder="host" />
        <input class="text-input mt-2" :value="turn.endpoint?.port || ''" @input="setTurnPort($event.target.value)" placeholder="port" inputmode="numeric" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">VK link (основная)</label>
        <textarea class="text-input" rows="2" :value="turn.link || ''" @input="setTurn('link', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Дополнительные VK ссылки</label>
        <div v-for="(link, idx) in turnLinks" :key="idx" class="vk-link-row">
          <textarea
            class="text-input"
            rows="2"
            :value="link"
            @input="updateTurnLink(idx, $event.target.value)"
            placeholder="https://vk.com/..."
          />
          <button class="icon-button" type="button" @click="removeTurnLink(idx)" title="Удалить">
            <Trash2 class="button-icon" aria-hidden="true" />
          </button>
        </div>
        <div class="actions-row mt-2">
          <button class="button-secondary" type="button" @click="addTurnLink">
            <Plus class="button-icon" aria-hidden="true" />
            <span>Добавить ссылку</span>
          </button>
        </div>
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Резервная VK ссылка</label>
        <textarea class="text-input" rows="2" :value="turn.linkSecondary || ''" @input="setTurn('linkSecondary', $event.target.value)" />
      </div>
      <div class="form-row">
        <label class="form-label">Threads</label>
        <input class="text-input form-input-narrow" :value="turn.threads || ''" @input="setTurn('threads', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Session mode</label>
        <OneuiSelect :model-value="turn.sessionMode || 'TURN_SESSION_MODE_UNSPECIFIED'" :options="sessionModeOptions" @change="setTurn('sessionMode', $event === 'TURN_SESSION_MODE_UNSPECIFIED' ? undefined : $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">UDP</label>
        <OneuiSwitch :model-value="!!turn.useUdp" @change="setTurn('useUdp', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Без обфускации</label>
        <OneuiSwitch :model-value="!!turn.noObfuscation" @change="setTurn('noObfuscation', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Свои DNS-резолверы</label>
        <textarea
          class="text-input"
          rows="3"
          :value="turnUserDnsText"
          @input="setTurnUserDns($event.target.value)"
          placeholder="https://dns.example/dns-query&#10;udp://77.88.8.8:53&#10;77.88.8.8"
        />
        <p class="form-hint">
          По одной записи на строку. Ставятся ПЕРЕД встроенным списком (Yandex → Google → Cloudflare).
          DoH (https://...), plain UDP (udp://ip[:port] или просто ip[:port]). DoT пока не поддерживается.
        </p>
      </div>
    </section>

    <!-- Xray basics -->
    <section v-if="show('xray')" class="form-section">
      <h3 class="form-section-title">Xray</h3>
      <div class="form-row">
        <label class="form-label">Allow LAN</label>
        <OneuiSwitch :model-value="!!xraySettings.allowLan" @change="setXrayS('allowLan', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Allow insecure</label>
        <OneuiSwitch :model-value="!!xraySettings.allowInsecure" @change="setXrayS('allowInsecure', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">IPv6</label>
        <OneuiSwitch :model-value="!!xraySettings.ipv6" @change="setXrayS('ipv6', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Sniffing</label>
        <OneuiSwitch :model-value="!!xraySettings.sniffingEnabled" @change="setXrayS('sniffingEnabled', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Remote DNS</label>
        <input class="text-input" :value="xraySettings.remoteDns || ''" @input="setXrayS('remoteDns', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Direct DNS</label>
        <input class="text-input" :value="xraySettings.directDns || ''" @input="setXrayS('directDns', $event.target.value)" />
      </div>
    </section>

    <!-- WB Stream -->
    <section v-if="show('wb_stream')" class="form-section">
      <h3 class="form-section-title">WB Stream</h3>
      <div class="form-row">
        <label class="form-label">Под-backend</label>
        <OneuiSelect :model-value="wb.tunnelMode || 'TUNNEL_MODE_WIREGUARD'" :options="tunnelModeOptions" @change="setWb('tunnelMode', $event === 'TUNNEL_MODE_WIREGUARD' ? undefined : $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Display name</label>
        <input class="text-input" :value="wb.displayName || ''" @input="setWb('displayName', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Room ID</label>
        <input class="text-input" :value="wb.roomId || ''" @input="setWb('roomId', $event.target.value)" />
      </div>
      <div class="form-row">
        <label class="form-label">Обмен room data через VK TURN</label>
        <OneuiSwitch :model-value="!!wb.exchangeViaVkTurn" @change="setWb('exchangeViaVkTurn', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">E2E enabled</label>
        <OneuiSwitch :model-value="!!wb.e2eEnabled" @change="setWb('e2eEnabled', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">E2E ключ (32 байта, base64)</label>
        <input class="text-input" :value="wb.e2eSecret || ''" @input="setWb('e2eSecret', $event.target.value)" />
        <div class="actions-row mt-2">
          <button class="button-secondary" type="button" @click="generateE2ESecret">
            <span>Сгенерировать ключ</span>
          </button>
        </div>
      </div>
      <div class="form-row">
        <label class="form-label">Параллельных комнат</label>
        <input class="text-input form-input-narrow" :value="wb.roomCount || ''" @input="setWb('roomCount', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
    </section>

    <!-- Backend selector -->
    <section v-if="show('backend')" class="form-section">
      <h3 class="form-section-title">Бэкенд</h3>
      <div class="form-row">
        <label class="form-label">Активный backend</label>
        <OneuiSelect :model-value="modelValue.backend || 'BACKEND_TYPE_UNSPECIFIED'" :options="backendOptions" @change="setRoot('backend', $event === 'BACKEND_TYPE_UNSPECIFIED' ? undefined : $event)" />
      </div>
    </section>

    <!-- WireGuard -->
    <section v-if="show('wireguard')" class="form-section">
      <h3 class="form-section-title">WireGuard</h3>
      <div class="form-row form-row-stack">
        <label class="form-label">Endpoint host</label>
        <input class="text-input" :value="wg.endpoint?.host || ''" @input="setWgEndpointHost($event.target.value)" />
      </div>
      <div class="form-row">
        <label class="form-label">Endpoint port</label>
        <input class="text-input form-input-narrow" :value="wg.endpoint?.port || ''" @input="setWgEndpointPort($event.target.value)" inputmode="numeric" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Interface DNS (через запятую)</label>
        <input class="text-input" :value="(wg.iface?.dns || []).join(', ')" @input="setWgIfaceArray('dns', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Interface адреса (через запятую)</label>
        <input class="text-input" :value="(wg.iface?.addrs || []).join(', ')" @input="setWgIfaceArray('addrs', $event.target.value)" />
      </div>
      <div class="form-row">
        <label class="form-label">MTU</label>
        <input class="text-input form-input-narrow" :value="wg.iface?.mtu || ''" @input="setWgIfaceField('mtu', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Interface private key (base64)</label>
        <input class="text-input" :value="wg.iface?.privateKey || ''" @input="setWgIfaceField('privateKey', $event.target.value)" />
      </div>

      <h4 class="form-subsection-title">Peer</h4>
      <div class="form-row form-row-stack">
        <label class="form-label">Public key (base64)</label>
        <input class="text-input" :value="wg.peer?.publicKey || ''" @input="setWgPeerField('publicKey', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Preshared key (base64)</label>
        <input class="text-input" :value="wg.peer?.presharedKey || ''" @input="setWgPeerField('presharedKey', $event.target.value)" />
      </div>
    </section>

    <!-- AmneziaWG -->
    <section v-if="show('amneziawg')" class="form-section">
      <h3 class="form-section-title">AmneziaWG</h3>
      <div class="form-row form-row-stack">
        <label class="form-label">awg-quick конфиг</label>
        <textarea class="text-input admin-config-area" rows="10" spellcheck="false" :value="awg.awgQuickConfig || ''" @input="setAwg('awgQuickConfig', $event.target.value)" />
      </div>
    </section>

    <!-- App routing -->
    <section v-if="show('app_routing')" class="form-section">
      <h3 class="form-section-title">Per-app routing</h3>
      <div class="form-row">
        <label class="form-label">Режим bypass</label>
        <OneuiSwitch :model-value="!!appRouting.bypass" @change="setAppRouting('bypass', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Пакеты (через запятую или с новой строки)</label>
        <textarea class="text-input" rows="5" spellcheck="false" :value="(appRouting.packages || []).join('\n')" @input="setAppRoutingArray('packages', $event.target.value)" />
      </div>
    </section>

    <!-- Xposed -->
    <section v-if="show('xposed')" class="form-section">
      <h3 class="form-section-title">Xposed</h3>
      <div class="form-row">
        <label class="form-label">Enabled</label>
        <OneuiSwitch :model-value="!!xposed.enabled" @change="setXposed('enabled', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">All apps</label>
        <OneuiSwitch :model-value="!!xposed.allApps" @change="setXposed('allApps', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Native hook</label>
        <OneuiSwitch :model-value="!!xposed.nativeHookEnabled" @change="setXposed('nativeHookEnabled', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Inline hooks</label>
        <OneuiSwitch :model-value="!!xposed.inlineHooksEnabled" @change="setXposed('inlineHooksEnabled', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Hide VPN apps</label>
        <OneuiSwitch :model-value="!!xposed.hideVpnApps" @change="setXposed('hideVpnApps', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Hide from dumpsys</label>
        <OneuiSwitch :model-value="!!xposed.hideFromDumpsys" @change="setXposed('hideFromDumpsys', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Procfs hook</label>
        <OneuiSelect :model-value="xposed.procfsHookMode || 'XPOSED_PROCFS_HOOK_MODE_UNSPECIFIED'" :options="procfsOptions" @change="setXposed('procfsHookMode', $event === 'XPOSED_PROCFS_HOOK_MODE_UNSPECIFIED' ? undefined : $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">ICMP spoofing</label>
        <OneuiSelect :model-value="xposed.icmpSpoofingMode || 'XPOSED_ICMP_SPOOFING_MODE_UNSPECIFIED'" :options="icmpOptions" @change="setXposed('icmpSpoofingMode', $event === 'XPOSED_ICMP_SPOOFING_MODE_UNSPECIFIED' ? undefined : $event)" />
      </div>
    </section>

    <!-- Root settings -->
    <section v-if="show('root')" class="form-section">
      <h3 class="form-section-title">Root</h3>
      <div class="form-row">
        <label class="form-label">Root mode enabled</label>
        <OneuiSwitch :model-value="!!root.enabled" @change="setRootSettings('enabled', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Kernel WireGuard</label>
        <OneuiSwitch :model-value="!!root.kernelWireguard" @change="setRootSettings('kernelWireguard', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Xray TPROXY mode</label>
        <OneuiSwitch :model-value="!!root.xrayTproxyMode" @change="setRootSettings('xrayTproxyMode', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">WG-интерфейс имя (template)</label>
        <input class="text-input" :value="root.wgInterfaceName || ''" @input="setRootSettings('wgInterfaceName', $event.target.value)" />
      </div>
    </section>

    <!-- Sharing -->
    <section v-if="show('sharing')" class="form-section">
      <h3 class="form-section-title">Sharing</h3>
      <div class="form-row">
        <label class="form-label">Автозапуск раздачи</label>
        <OneuiSwitch :model-value="!!sharing.autoStartOnBoot" @change="setSharing('autoStartOnBoot', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Disable IPv6</label>
        <OneuiSwitch :model-value="!!sharing.disableIpv6" @change="setSharing('disableIpv6', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">DHCP workaround</label>
        <OneuiSwitch :model-value="!!sharing.dhcpWorkaround" @change="setSharing('dhcpWorkaround', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Upstream interface (вручную)</label>
        <input class="text-input" :value="sharing.upstreamInterface || ''" @input="setSharing('upstreamInterface', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Fallback upstream interface</label>
        <input class="text-input" :value="sharing.fallbackUpstreamInterface || ''" @input="setSharing('fallbackUpstreamInterface', $event.target.value)" />
      </div>
    </section>

    <!-- ByeDPI -->
    <section v-if="show('byedpi')" class="form-section">
      <h3 class="form-section-title">ByeDPI</h3>

      <div class="form-row">
        <label class="form-label">Enabled</label>
        <OneuiSwitch :model-value="!!byeDpi.enabled" @change="setByeDpi('enabled', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Авто-старт с Xray</label>
        <OneuiSwitch :model-value="!!byeDpi.autoStartWithXray" @change="setByeDpi('autoStartWithXray', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Использовать сырые аргументы (cmd_args)</label>
        <OneuiSwitch :model-value="!!byeDpi.useCommandSettings" @change="setByeDpi('useCommandSettings', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Сырые аргументы (cmd_args)</label>
        <textarea class="text-input admin-config-area" rows="4" spellcheck="false" :value="byeDpi.cmdArgs || ''" @input="setByeDpi('cmdArgs', $event.target.value)" placeholder="--desync-method split --split-position 2" />
      </div>

      <h4 class="form-subsection-title">Локальный прокси</h4>
      <div class="form-row form-row-stack">
        <label class="form-label">IP</label>
        <input class="text-input" :value="byeDpi.proxyIp || ''" @input="setByeDpi('proxyIp', $event.target.value)" placeholder="127.0.0.1" />
      </div>
      <div class="form-row">
        <label class="form-label">Порт</label>
        <input class="text-input form-input-narrow" :value="byeDpi.proxyPort || ''" @input="setByeDpi('proxyPort', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Auth</label>
        <OneuiSwitch :model-value="!!byeDpi.proxyAuthEnabled" @change="setByeDpi('proxyAuthEnabled', $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Имя пользователя</label>
        <input class="text-input" :value="byeDpi.proxyUsername || ''" @input="setByeDpi('proxyUsername', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Пароль</label>
        <input class="text-input" :value="byeDpi.proxyPassword || ''" @input="setByeDpi('proxyPassword', $event.target.value)" />
      </div>

      <h4 class="form-subsection-title">Сеть</h4>
      <div class="form-row">
        <label class="form-label">Max connections</label>
        <input class="text-input form-input-narrow" :value="byeDpi.maxConnections || ''" @input="setByeDpi('maxConnections', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Buffer size</label>
        <input class="text-input form-input-narrow" :value="byeDpi.bufferSize || ''" @input="setByeDpi('bufferSize', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">No domain</label>
        <OneuiSwitch :model-value="!!byeDpi.noDomain" @change="setByeDpi('noDomain', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">TCP Fast Open</label>
        <OneuiSwitch :model-value="!!byeDpi.tcpFastOpen" @change="setByeDpi('tcpFastOpen', $event)" />
      </div>

      <h4 class="form-subsection-title">Hosts mode</h4>
      <div class="form-row">
        <label class="form-label">Mode</label>
        <OneuiSelect :model-value="byeDpi.hostsMode || 'BYEDPI_HOSTS_MODE_UNSPECIFIED'" :options="byedpiHostsOptions" @change="setByeDpi('hostsMode', $event === 'BYEDPI_HOSTS_MODE_UNSPECIFIED' ? undefined : $event)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Blacklist (через запятую)</label>
        <textarea class="text-input" rows="3" spellcheck="false" :value="byeDpi.hostsBlacklist || ''" @input="setByeDpi('hostsBlacklist', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Whitelist (через запятую)</label>
        <textarea class="text-input" rows="3" spellcheck="false" :value="byeDpi.hostsWhitelist || ''" @input="setByeDpi('hostsWhitelist', $event.target.value)" />
      </div>

      <h4 class="form-subsection-title">Desync</h4>
      <div class="form-row">
        <label class="form-label">Method</label>
        <OneuiSelect :model-value="byeDpi.desyncMethod || 'BYEDPI_DESYNC_METHOD_UNSPECIFIED'" :options="byedpiDesyncOptions" @change="setByeDpi('desyncMethod', $event === 'BYEDPI_DESYNC_METHOD_UNSPECIFIED' ? undefined : $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Default TTL</label>
        <input class="text-input form-input-narrow" :value="byeDpi.defaultTtl || ''" @input="setByeDpi('defaultTtl', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Split position</label>
        <input class="text-input form-input-narrow" :value="byeDpi.splitPosition || ''" @input="setByeDpi('splitPosition', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Split at host</label>
        <OneuiSwitch :model-value="!!byeDpi.splitAtHost" @change="setByeDpi('splitAtHost', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Drop SACK</label>
        <OneuiSwitch :model-value="!!byeDpi.dropSack" @change="setByeDpi('dropSack', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Fake TTL</label>
        <input class="text-input form-input-narrow" :value="byeDpi.fakeTtl || ''" @input="setByeDpi('fakeTtl', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Fake offset</label>
        <input class="text-input form-input-narrow" :value="byeDpi.fakeOffset || ''" @input="setByeDpi('fakeOffset', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">Fake SNI</label>
        <input class="text-input" :value="byeDpi.fakeSni || ''" @input="setByeDpi('fakeSni', $event.target.value)" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">OOB data</label>
        <input class="text-input" :value="byeDpi.oobData || ''" @input="setByeDpi('oobData', $event.target.value)" />
      </div>
      <div class="form-row">
        <label class="form-label">Desync HTTP</label>
        <OneuiSwitch :model-value="!!byeDpi.desyncHttp" @change="setByeDpi('desyncHttp', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Desync HTTPS</label>
        <OneuiSwitch :model-value="!!byeDpi.desyncHttps" @change="setByeDpi('desyncHttps', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Desync UDP</label>
        <OneuiSwitch :model-value="!!byeDpi.desyncUdp" @change="setByeDpi('desyncUdp', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Host mixed case</label>
        <OneuiSwitch :model-value="!!byeDpi.hostMixedCase" @change="setByeDpi('hostMixedCase', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Domain mixed case</label>
        <OneuiSwitch :model-value="!!byeDpi.domainMixedCase" @change="setByeDpi('domainMixedCase', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">Host remove spaces</label>
        <OneuiSwitch :model-value="!!byeDpi.hostRemoveSpaces" @change="setByeDpi('hostRemoveSpaces', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">TLSrec</label>
        <OneuiSwitch :model-value="!!byeDpi.tlsrecEnabled" @change="setByeDpi('tlsrecEnabled', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">TLSrec position</label>
        <input class="text-input form-input-narrow" :value="byeDpi.tlsrecPosition || ''" @input="setByeDpi('tlsrecPosition', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">TLSrec at SNI</label>
        <OneuiSwitch :model-value="!!byeDpi.tlsrecAtSni" @change="setByeDpi('tlsrecAtSni', $event)" />
      </div>
      <div class="form-row">
        <label class="form-label">UDP fake count</label>
        <input class="text-input form-input-narrow" :value="byeDpi.udpFakeCount || ''" @input="setByeDpi('udpFakeCount', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>

      <h4 class="form-subsection-title">Proxytest</h4>
      <div class="form-row">
        <label class="form-label">Delay (ms)</label>
        <input class="text-input form-input-narrow" :value="byeDpi.proxytestDelay || ''" @input="setByeDpi('proxytestDelay', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Requests</label>
        <input class="text-input form-input-narrow" :value="byeDpi.proxytestRequests || ''" @input="setByeDpi('proxytestRequests', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Limit</label>
        <input class="text-input form-input-narrow" :value="byeDpi.proxytestLimit || ''" @input="setByeDpi('proxytestLimit', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row">
        <label class="form-label">Timeout (ms)</label>
        <input class="text-input form-input-narrow" :value="byeDpi.proxytestTimeout || ''" @input="setByeDpi('proxytestTimeout', toIntOrUndef($event.target.value))" inputmode="numeric" />
      </div>
      <div class="form-row form-row-stack">
        <label class="form-label">SNI</label>
        <input class="text-input" :value="byeDpi.proxytestSni || ''" @input="setByeDpi('proxytestSni', $event.target.value)" />
      </div>
      <div class="form-row">
        <label class="form-label">Кастомные стратегии</label>
        <OneuiSwitch :model-value="!!byeDpi.proxytestUseCustomStrategies" @change="setByeDpi('proxytestUseCustomStrategies', $event)" />
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed } from "vue";
import { Plus, Trash2 } from "lucide-vue-next";
import OneuiSwitch from "@/components/controls/OneuiSwitch.vue";
import OneuiSelect from "@/components/controls/OneuiSelect.vue";

const props = defineProps({
  modelValue: { type: Object, required: true },
  // Optional whitelist of section ids — if omitted, all are rendered.
  sections: { type: Array, default: null },
  // Whether the target client has root grant. Root/sharing/xposed sections are
  // hidden when this is false so we don't offer settings that would freeze a
  // non-rooted device on import.
  hasRootAccess: { type: Boolean, default: false },
});
const emit = defineEmits(["update:modelValue"]);

const ROOT_ONLY_SECTIONS = new Set(["root", "sharing", "xposed"]);

function show(id) {
  if (ROOT_ONLY_SECTIONS.has(id) && !props.hasRootAccess) {
    return false;
  }
  return !props.sections || props.sections.includes(id);
}

const themeOptions = [
  { value: "THEME_MODE_UNSPECIFIED", label: "Системная (по умолчанию)" },
  { value: "THEME_MODE_LIGHT", label: "Светлая" },
  { value: "THEME_MODE_DARK", label: "Тёмная" },
  { value: "THEME_MODE_SYSTEM", label: "Системная" },
];

const dnsOptions = [
  { value: "DNS_MODE_UNSPECIFIED", label: "По умолчанию (auto)" },
  { value: "DNS_MODE_AUTO", label: "Авто (UDP → DoH fallback)" },
  { value: "DNS_MODE_UDP", label: "Только UDP/53" },
  { value: "DNS_MODE_DOH", label: "Только DoH" },
];

const sessionModeOptions = [
  { value: "TURN_SESSION_MODE_UNSPECIFIED", label: "Не задан" },
  { value: "TURN_SESSION_MODE_AUTO", label: "Auto" },
  { value: "TURN_SESSION_MODE_MAINLINE", label: "Mainline" },
  { value: "TURN_SESSION_MODE_MUX", label: "MU" },
];

// Top-level выбор. Legacy proto-значения (VK_TURN_WIREGUARD, AMNEZIAWG,
// AMNEZIAWG_PLAIN) теперь представлены через сочетание top-level + sub-backend:
// VK TURN → BACKEND_TYPE_VK_TURN_WIREGUARD (+ Turn.tunnelMode для AWG-варианта),
// AmneziaWG plain → BACKEND_TYPE_AMNEZIAWG_PLAIN, WB Stream → BACKEND_TYPE_WB_STREAM
// (+ WbStream.tunnelMode). Drop-down содержит 5 видимых опций; маппинг при save
// делает topLevelToBackend ниже.
const backendOptions = [
  { value: "BACKEND_TYPE_UNSPECIFIED", label: "Не задан" },
  { value: "BACKEND_TYPE_VK_TURN_WIREGUARD", label: "VK TURN" },
  { value: "BACKEND_TYPE_WB_STREAM", label: "WB Stream" },
  { value: "BACKEND_TYPE_WIREGUARD", label: "WireGuard" },
  { value: "BACKEND_TYPE_AMNEZIAWG_PLAIN", label: "AmneziaWG" },
  { value: "BACKEND_TYPE_XRAY", label: "Xray" },
];

const tunnelModeOptions = [
  { value: "TUNNEL_MODE_WIREGUARD", label: "WireGuard" },
  { value: "TUNNEL_MODE_AMNEZIAWG", label: "AmneziaWG" },
];

const procfsOptions = [
  { value: "XPOSED_PROCFS_HOOK_MODE_UNSPECIFIED", label: "По умолчанию" },
  { value: "XPOSED_PROCFS_HOOK_MODE_DISABLED", label: "Отключено" },
  { value: "XPOSED_PROCFS_HOOK_MODE_FILTER", label: "Фильтр" },
  { value: "XPOSED_PROCFS_HOOK_MODE_NO_ACCESS", label: "No access" },
  { value: "XPOSED_PROCFS_HOOK_MODE_FILE_NOT_FOUND", label: "File not found" },
];

const icmpOptions = [
  { value: "XPOSED_ICMP_SPOOFING_MODE_UNSPECIFIED", label: "По умолчанию" },
  { value: "XPOSED_ICMP_SPOOFING_MODE_DISABLED", label: "Отключено" },
  { value: "XPOSED_ICMP_SPOOFING_MODE_PING_NOT_FOUND", label: "Ping not found" },
  { value: "XPOSED_ICMP_SPOOFING_MODE_EMPTY_RESPONSE", label: "Empty response" },
];

const byedpiHostsOptions = [
  { value: "BYEDPI_HOSTS_MODE_UNSPECIFIED", label: "По умолчанию" },
  { value: "BYEDPI_HOSTS_MODE_DISABLE", label: "Disable" },
  { value: "BYEDPI_HOSTS_MODE_BLACKLIST", label: "Blacklist" },
  { value: "BYEDPI_HOSTS_MODE_WHITELIST", label: "Whitelist" },
];

const byedpiDesyncOptions = [
  { value: "BYEDPI_DESYNC_METHOD_UNSPECIFIED", label: "По умолчанию" },
  { value: "BYEDPI_DESYNC_METHOD_NONE", label: "None" },
  { value: "BYEDPI_DESYNC_METHOD_SPLIT", label: "Split" },
  { value: "BYEDPI_DESYNC_METHOD_DISORDER", label: "Disorder" },
  { value: "BYEDPI_DESYNC_METHOD_FAKE", label: "Fake" },
  { value: "BYEDPI_DESYNC_METHOD_OOB", label: "OOB" },
  { value: "BYEDPI_DESYNC_METHOD_DISOOB", label: "DisOOB" },
];

const ap = computed(() => props.modelValue.appPreferences || {});
const turn = computed(() => props.modelValue.turn || {});
const turnLinks = computed(() => turn.value.links || []);
// userDns в proto — repeated string. Привязываем как многострочный текст
// (одна запись на строку), на сохранении сплитим по , ; \n \r и trim'им.
const turnUserDnsText = computed(() => (turn.value.userDns || []).join("\n"));
const xraySettings = computed(() => props.modelValue.xray?.settings || {});
const wb = computed(() => props.modelValue.wbStream || {});
const sharing = computed(() => props.modelValue.sharing || {});
const wg = computed(() => props.modelValue.wg || {});
const awg = computed(() => props.modelValue.awg || {});
const appRouting = computed(() => props.modelValue.appRouting || {});
const xposed = computed(() => props.modelValue.xposed || {});
const root = computed(() => props.modelValue.root || {});
const byeDpi = computed(() => props.modelValue.byeDpi || {});

function emitMerge(patch) {
  emit("update:modelValue", { ...props.modelValue, ...patch });
}

function setAp(key, value) {
  const next = { ...ap.value };
  if (value === undefined) delete next[key];
  else next[key] = value;
  emitMerge({ appPreferences: next });
}

function setTurn(key, value) {
  const next = { ...turn.value };
  if (value === undefined || value === "" || value === null) delete next[key];
  else next[key] = value;
  emitMerge({ turn: next });
}

function updateTurnLink(idx, value) {
  const links = [...(turn.value.links || [])];
  links[idx] = value;
  setTurn("links", links.filter((s) => s != null));
}

function addTurnLink() {
  const links = [...(turn.value.links || []), ""];
  setTurn("links", links);
}

function removeTurnLink(idx) {
  const links = [...(turn.value.links || [])];
  links.splice(idx, 1);
  setTurn("links", links.length ? links : undefined);
}

function setTurnUserDns(value) {
  const entries = String(value || "")
    .split(/[\n\r,;]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  setTurn("userDns", entries.length ? entries : undefined);
}

function setTurnHost(host) {
  const ep = { ...(turn.value.endpoint || {}), host };
  if (!host) delete ep.host;
  setTurn("endpoint", Object.keys(ep).length ? ep : undefined);
}

function setTurnPort(portText) {
  const port = portText === "" ? undefined : Number(portText);
  const ep = { ...(turn.value.endpoint || {}) };
  if (port == null || Number.isNaN(port)) delete ep.port;
  else ep.port = port;
  setTurn("endpoint", Object.keys(ep).length ? ep : undefined);
}

function setXrayS(key, value) {
  const next = { ...xraySettings.value };
  if (value === undefined || value === "" || value === null) delete next[key];
  else next[key] = value;
  const xray = { ...(props.modelValue.xray || {}), settings: next };
  emitMerge({ xray });
}

function setWb(key, value) {
  const next = { ...wb.value };
  if (value === undefined || value === "" || value === null) delete next[key];
  else next[key] = value;
  emitMerge({ wbStream: next });
}

function setSharing(key, value) {
  const next = { ...sharing.value };
  if (value === undefined || value === "" || value === null) delete next[key];
  else next[key] = value;
  emitMerge({ sharing: next });
}

function toIntOrUndef(text) {
  if (text === "" || text == null) return undefined;
  const n = Number(text);
  return Number.isFinite(n) ? n : undefined;
}

function setRoot(key, value) {
  const next = { ...props.modelValue };
  if (value === undefined || value === "" || value === null) delete next[key];
  else next[key] = value;
  emit("update:modelValue", next);
}

function setSubObj(parentKey, key, value) {
  const sub = { ...(props.modelValue[parentKey] || {}) };
  if (value === undefined || value === "" || value === null) delete sub[key];
  else sub[key] = value;
  emitMerge({ [parentKey]: Object.keys(sub).length ? sub : undefined });
}

function setWgIfaceField(key, value) {
  const iface = { ...(wg.value.iface || {}) };
  if (value === undefined || value === "" || value === null) delete iface[key];
  else iface[key] = value;
  const next = { ...(wg.value || {}), iface: Object.keys(iface).length ? iface : undefined };
  emitMerge({ wg: next });
}

function setWgIfaceArray(key, csvText) {
  const arr = csvText
    .split(/[,\n]/)
    .map((s) => s.trim())
    .filter(Boolean);
  setWgIfaceField(key, arr.length ? arr : undefined);
}

function setWgEndpointHost(host) {
  const ep = { ...(wg.value.endpoint || {}), host };
  if (!host) delete ep.host;
  const next = { ...(wg.value || {}), endpoint: Object.keys(ep).length ? ep : undefined };
  emitMerge({ wg: next });
}

function setWgPeerField(key, value) {
  const peer = { ...(wg.value.peer || {}) };
  if (value === undefined || value === "" || value === null) delete peer[key];
  else peer[key] = value;
  const next = { ...(wg.value || {}), peer: Object.keys(peer).length ? peer : undefined };
  emitMerge({ wg: next });
}

function setWgEndpointPort(portText) {
  const port = portText === "" ? undefined : Number(portText);
  const ep = { ...(wg.value.endpoint || {}) };
  if (port == null || Number.isNaN(port)) delete ep.port;
  else ep.port = port;
  const next = { ...(wg.value || {}), endpoint: Object.keys(ep).length ? ep : undefined };
  emitMerge({ wg: next });
}

function setAwg(key, value) {
  setSubObj("awg", key, value);
}

function setAppRouting(key, value) {
  setSubObj("appRouting", key, value);
}

function setAppRoutingArray(key, text) {
  const arr = text
    .split(/[,\n]/)
    .map((s) => s.trim())
    .filter(Boolean);
  setAppRouting(key, arr.length ? arr : undefined);
}

function setXposed(key, value) {
  setSubObj("xposed", key, value);
}

function setRootSettings(key, value) {
  setSubObj("root", key, value);
}

function setByeDpi(key, value) {
  setSubObj("byeDpi", key, value);
}

function generateE2ESecret() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  // protojson encodes `bytes` as base64 (standard, padded). Match that so the
  // client deserialises the same way it does for an exported wingsv:// link.
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  const base64 = btoa(binary);
  setWb("e2eSecret", base64);
}
</script>
