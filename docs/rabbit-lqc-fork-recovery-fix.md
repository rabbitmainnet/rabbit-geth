# Rabbit Chain — correção de recuperação de fork LQC

## Evidência do laboratório

Durante o teste com 20 produtores, sete nós foram desligados no bloco 191. Os
13 nós ativos avançaram juntos e confirmaram uma transação no bloco 195. Ao
retornar, cada nó desligado produziu um ramo próprio a partir do bloco 192.

O sincronizador LQC anterior solicitava somente o próximo cabeçalho por número.
Como o bloco 192 local pertencia a outro ramo, o cabeçalho remoto 193 não tinha
um ancestral conhecido. Os logs registraram repetidamente `unknown ancestor`.
Além disso, construir `types.NewBlockWithHeader` descartava corpos, transações e
receipts.

## Correção

- o anúncio `BlockRangeUpdate` seleciona uma cadeia de altura maior;
- empates de altura usam o menor hash como desempate determinístico;
- o cabeçalho-alvo é solicitado pelo hash anunciado;
- `downloader.BeaconSync` em modo full encontra o ancestral comum, baixa os
  cabeçalhos e corpos e executa a reorganização canônica;
- somente uma recuperação LQC é iniciada por vez;
- `lqcv2.VerifyHeaders` agora executa `VerifyHeader` para cada cabeçalho do
  lote, impedindo que o backfill ignore a validação do produtor.

A correção não altera reward, committee, vesting, halving nem os arquivos de
genesis. A ausência de assinatura criptográfica dos blocos da engine ativa
`lqcv2` permanece como achado arquitetural separado e bloqueia uma liberação de
mainnet até ser resolvida ou até a ativação auditada da engine LQC definitiva.
