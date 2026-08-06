# oci-backup-healer

Daemon em Go para failover automático de Block Volumes de backup e contingência ativa entre regiões da Oracle Cloud Infrastructure (OCI).

## 🚀 Arquitetura e Recursos
*   **Monitoramento Contínuo**: Checa a saúde da VM primária na API oficial do Compute da OCI.
*   **Failover Idempotente**: Desassocia o Block Volume da VM1 e associa na VM2 de forma atômica em caso de indisponibilidade.
*   **Mapeamento Físico**: Roda comandos do shell Linux local (`iscsiadm`, `mount`) para conectar o disco via iSCSI.
*   **Alertas ntfy**: Emite notificações estruturadas com prioridade e tags diretamente no celular.

## 🛠️ Como Compilar
```bash
make build
```
