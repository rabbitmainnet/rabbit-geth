# Rabbit Chain — correção de relay e raiz de estado com transações

## Evidência do laboratório

O laboratório com 20 produtores continuou avançando normalmente e todos os nós mantiveram
19 peers. Uma transferência EIP-1559 assinada corretamente foi aceita no txpool do node20,
mas não apareceu nos outros 19 txpools. Quando o node20 tentou montar um bloco contendo a
transação, o próprio cliente rejeitou o bloco com `invalid merkle root`. Todos os blocos
canônicos permaneceram com zero transações.

## Causa 1: relay de transações desativado

O loop de produção LQC importa blocos diretamente e não conclui o ciclo tradicional do
downloader Ethereum. O protocolo ETH, portanto, mantinha `AcceptTxs` desativado e descartava
transações recebidas dos peers.

`eth/backend.go` agora aguarda um cabeçalho LQC recente e diferente do genesis antes de marcar
os recursos pós-sincronização como prontos. Isso ativa o recebimento e a propagação de
transações sem aceitar tráfego durante um banco vazio ou uma cadeia antiga ainda parada.

## Causa 2: destinatários de taxa diferentes

O engine LQC grava o produtor canônico em `header.Coinbase`. Um nó fallback pode construir o
bloco usando outro endereço local. Antes da correção, o EVM do construtor creditava a priority
fee ao endereço local, mas a reexecução do bloco creditava a taxa ao produtor do cabeçalho.
Os dois estados terminavam com raízes diferentes.

`miner/worker.go` agora usa `header.Coinbase` como destinatário de taxa sempre que a chain
possui configuração LQC. Redes sem LQC preservam o comportamento original do geth.

## Testes de regressão

- `miner/rabbit_lqc_fee_recipient_test.go` valida o destinatário LQC e preserva o comportamento
  das demais redes.
- `eth/rabbit_lqc_sync_test.go` valida que genesis, cabeçalho antigo e horário futuro inválido
  não habilitam relay; um cabeçalho LQC recente habilita.
- Os mocks legados em `miner/miner_test.go` e `miner/payload_building_test.go` agora implementam
  `AccountManager()`, que já fazia parte da interface de produção, permitindo que todo o pacote
  `miner` volte a compilar durante os testes.

Nenhum arquivo em `consensus/lqc`, `consensus/lqcv2`, `core/vesting` ou `networks` foi alterado
por esta correção.
