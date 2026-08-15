# Rabbit Chain — ativação bootstrap permissionless 1.0.0

## Falha observada

O laboratório iniciou 21 nós e estabeleceu todos os peers, mas permaneceu na
altura zero. Os 20 produtores bootstrap foram classificados fora da fila no
bloco 1.

## Causa

`activationDelay` protege participantes que entram por uma operação REGISTER.
Os participantes bootstrap do genesis também estavam recebendo esse atraso.
Com o protocolo ativado no bloco 1 e atraso igual a 2, a cadeia não possuía um
produtor elegível para criar o primeiro bloco.

## Correção

Participantes bootstrap são identificados canonicamente pela sequência zero.
Operações assinadas nunca podem usar sequência zero. Por isso os bootstraps do
genesis ficam imediatamente elegíveis, enquanto participantes cadastrados por
REGISTER continuam obedecendo `N+1+activationDelay`.

## Escopo

- Não altera recompensa por bloco.
- Não altera committee ou divisão 70/30.
- Não altera reward locker, vesting, releases ou halving.
- Não remove LightHash, assinatura, sequência, heartbeat ou EXIT.
- Adiciona regressões determinísticas para ativação no bloco 1 com atraso 2.
