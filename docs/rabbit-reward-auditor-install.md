# Instalação do auditor no WSL

Este pacote contém somente arquivos novos do auditor. Ele não substitui nem modifica arquivos
em `consensus/lqc`.

Depois de baixar `rabbit-reward-auditor-archive-lab-1.2.1.tar.gz` no Windows, execute no WSL:

```bash
PACOTE="$(find /mnt/c/Users -type f -name 'rabbit-reward-auditor-archive-lab-1.2.1.tar.gz' -print -quit 2>/dev/null)"
tar -xzf "$PACOTE" -C ~/projects/rabbit-geth
cd ~/projects/rabbit-geth
./scripts/rabbit-devnet/run-reward-audit.sh
```

O laboratório pode permanecer rodando. O script cria `build/bin/rabbit-audit` e os relatórios
dentro de `audit-reports/AAAAmmdd-HHMMSS/`.

Se o status geral for `FAIL`, consulte as duas linhas seguintes no resumo. `Recompensas em
execução` informa se os blocos e o locker bateram; `Arquitetura do consenso` informa se ainda
existem bloqueios estruturais. O programa não altera o consenso.
