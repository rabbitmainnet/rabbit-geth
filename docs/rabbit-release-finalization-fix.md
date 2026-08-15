# Rabbit Chain — paridade de finalização e testes de fronteira

Data: 2026-08-05  
Base live aprovada: 20 blocos, 24 RAB emitidos e bloqueados, diferença zero.

## Problema

Na engine ativa `lqcv2`, `FinalizeAndAssemble` executava o release global durante a produção,
mas `Finalize` não o executava durante a importação/validação. No primeiro release, produtor
e validador poderiam obter estados diferentes.

Além disso, `Finalize` aplicava rewards somente quando `vm.StateDB` era exatamente
`*state.StateDB`. Quando o geth envolvia o estado com hooks de tracing, o type assertion
falhava e rewards/releases eram silenciosamente ignorados.

## Correção

- O release global e a distribuição passam a acontecer em `Finalize`, caminho comum aos dois
  fluxos.
- `FinalizeAndAssemble` chama `Finalize` uma única vez e calcula a raiz depois.
- O locker e o distribuidor aceitam `vm.StateDB`, inclusive o wrapper de tracing.
- A regra monetária, fila, committee, supply e alturas não foram alterados.

## Testes adicionados

- reward bloqueado no bloco 100.000;
- reward líquido no bloco 100.001;
- releases exatos de 25%, 50%, 75% e 100%, inclusive remainder;
- halvings exatos nas fronteiras das Eras 1, 2 e 3;
- release através de StateDB com tracing hooks;
- persistência do locker após limpeza EIP-158.

O teste de release usa valores pequenos com remainder para provar que a última etapa libera
todo o saldo e nenhum wei permanece preso.
