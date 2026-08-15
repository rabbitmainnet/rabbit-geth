# Rabbit Chain — auditoria de recompensas e revisão estática

Atualização: 2026-08-06  
Escopo: `consensus/lqc`, Reward Locker, emissão, transações, recuperação de fork e consolidação da engine.

## Evidência concluída antes da consolidação

O laboratório de 20 produtores aprovou:

- 273/273 blocos de recompensa, com `327,6 RAB` esperados e observados;
- diferença total de `0 wei` na emissão;
- divisão 70/30, committee, locker e índice de vesting;
- 125/125 transações EIP-1559 em cinco blocos e 20/20 nós;
- tips, base fee, burn, saldos e rejeições inválidas;
- parada e retorno de sete produtores;
- parada e reinício de 20/20 nós preservando a cadeia;
- recuperação determinística de fork;
- 120 verificações de fronteira para lock, releases e halving.

Essa evidência foi produzida enquanto a factory ainda iniciava `consensus/lqcv2`. Ela validou
o fluxo monetário compartilhado, mas também revelou que a engine ativa não era a versão
canônica escolhida para o projeto.

## Consolidação canônica

`eth/ethconfig.CreateConsensusEngine` agora instancia `consensus/lqc` para todo genesis com
`config.lqc`. O `consensus/lqcv2` permanece no repositório somente como implementação legada
e não participa da cadeia ativa.

Antes da ativação, o `lqc` foi consolidado com as correções já aprovadas:

- `core/vesting.CreditReward` e `ReleaseAllUnlockedRewards` no fluxo comum de finalização;
- proteção EIP-158 da conta de sistema do locker;
- resolução do produtor local usando a fila do próximo bloco;
- posição real do produtor/fallback devolvida ao minerador;
- validação de pais presentes no mesmo lote de headers durante sincronização;
- recuperação de fork e download de corpos, transações e receipts preservados;
- `fallbackCount` e `committeeSize` do genesis usados pela engine e pelo auditor.

O primeiro halving foi mantido exatamente no bloco `8.409.600`. `RabbitChainConfig`,
`RabbitDevnetChainConfig`, o genesis Rabbit mainnet e o genesis devnet estão alinhados nessa
mesma altura.

## Validação obrigatória após instalar o pacote

O banco produzido pela engine anterior não deve ser reaproveitado como evidência da nova
engine. A consolidação deve ser compilada e, em seguida, validada em um laboratório novo de
20 produtores. Os mesmos testes de recompensas, transações, carga, perda de nós, retorno e
reinício total devem ser repetidos antes de considerar a arquitetura aprovada.

O primeiro laboratório com a engine canônica revelou uma regressão importante: `Prepare`
substituía o horário real pelo mínimo do slot. Com o timestamp zero do genesis, todas as
janelas de fallback pareciam vencidas, os nós produziram rapidamente em forks diferentes e
o script antigo declarou um falso sucesso. A preparação agora preserva o horário real quando
ele já atende ao mínimo, existe um teste de regressão específico e o script só aprova após
verificar conectividade, diferença máxima de uma altura, hash comum e produtores distintos.

## Pontos ainda abertos

- A recompensa terminal de `0,15 RAB` continua indefinidamente, conforme a regra Era 3+.
- Como o genesis não recebe reward, a Era 0 paga os blocos 1 a 8.409.599. Essa semântica foi
  confirmada e mantida.
- O release global ainda percorre o índice de vesting. O custo linear deve ser medido antes
  de afirmar capacidade para milhões de mineradores.
- O registry público definitivo precisa de uma auditoria própria antes do genesis oficial;
  o laboratório bootstrap não prova entrada permissionless em escala pública.
