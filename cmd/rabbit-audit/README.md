# Rabbit Chain Reward Auditor

Auditor independente e somente leitura para a Rabbit Chain. O programa conecta-se ao nó por
IPC/HTTP/WebSocket, percorre a cadeia canônica e gera relatórios Markdown, JSON e CSV. Ele não
altera `consensus/lqc`, não envia transações e não para a mineração.

A versão 1.4 reconstrói o registry permissionless a partir dos headers canônicos. Assim, um
participante passa a ser acompanhado desde o bloco que contém seu `REGISTER`, antes mesmo de
produzir o primeiro bloco ou receber a primeira parcela do committee. Cada header também
valida o `registryRoot`, as operações, o produtor e a continuidade do snapshot.

## O que é verificado

- produtor e posição na fila determinística de cada bloco;
- REGISTER, HEARTBEAT e EXIT reconstruídos por hash de bloco;
- committee reconstruído com a fila canônica e o tamanho fixo ou dinâmico do genesis;
- divisão 70/30 e rateio do remainder sem perda de wei;
- recompensa de cada Era e limites exatos de halving;
- emissão esperada e observada;
- crédito líquido imediato de produtor e committee;
- armazenamento legado de vesting, que deve permanecer vazio e inalterado;
- efeitos de transações, isolados com `debug_traceBlockByNumber` quando necessário;
- divergências por bloco, carteira e papel (producer/committee).

## Laboratório de 20 produtores

Com o laboratório rodando, use a partir da raiz do repositório:

```bash
./scripts/rabbit-devnet/run-reward-audit.sh
```

O script compila somente `build/bin/rabbit-audit` e grava uma nova pasta em
`audit-reports/`. Além dos três relatórios, ele cria `binarios.txt`, com caminho, tamanho,
hash, linha de comando e metadados do `geth` em execução. Também cria `execucao.txt`, mesmo
quando a auditoria para antes dos relatórios. O código de saída `2` significa que a auditoria
encontrou uma divergência de recompensa ou um bloqueio crítico de arquitetura; os relatórios
continuam sendo produzidos normalmente.

O laboratório de 20 produtores inicia somente o node1 com `--gcmode archive` e
`--history.state 0`. Esse é o nó de auditoria: ele mantém o histórico necessário para comparar
qualquer bloco desde o genesis. Os outros 19 nós permanecem em modo full. Ao reiniciar o
laboratório, o diretório anterior é movido para um backup em vez de ser apagado.

## Uso direto

```bash
go build -o build/bin/rabbit-audit ./cmd/rabbit-audit

build/bin/rabbit-audit \
  --rpc /tmp/rabbit-20nodes/node1/geth.ipc \
  --genesis /tmp/rabbit-20nodes/genesis-runtime.json \
  --from 1 \
  --to 0 \
  --summary audit-reports/resumo.md \
  --json audit-reports/relatorio.json \
  --csv audit-reports/blocos.csv
```

`--to 0` fixa a auditoria na altura observada quando o programa começa, mesmo que novos
blocos continuem sendo produzidos durante a execução.

## Fonte de verdade

Para alturas posteriores à ativação de `registryProtocolBlock`, a lista estática de
`bootstrapParticipants` é usada somente para criar o snapshot inicial. Depois disso, todos os
participantes, filas e committees são derivados dos envelopes versionados nos headers. O
auditor rejeita qualquer quebra de continuidade ou `registryRoot` inválida.
