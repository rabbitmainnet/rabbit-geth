# Rabbit LQC — codec, pool e snapshots dos tickets

Status: **STORAGE_FOUNDATION_ONLY — mainnet bloqueada**

Esta etapa adiciona armazenamento e validação determinísticos para os tickets
sequenciais. Ela não coloca tickets no header ativo, não altera a escolha de
produtor, fallback ou committee e não modifica o genesis.

## Envelope canônico

- Prefixo binário e versão explícita.
- RLP canônico, no máximo 16 KiB.
- Até 64 tickets por lote.
- Compromisso com bloco, época, âncora e raiz pós-lote.
- Ordem independente da chegada na rede.
- Rejeição de duplicação, encoding alternativo e raiz falsa.

## Pool separado

- Não utiliza txpool/EVM.
- Capacidade global de 4.096 tickets.
- Máximo de 64 tickets pendentes por participante.
- Prova e assinatura verificadas antes da retenção.
- Seleção local em rodadas: uma lane profunda não ocupa o lote antes das
  demais lanes contínuas.
- O pool nunca é fonte de verdade do consenso.

## Snapshots

- Índice pelo hash do bloco, isolando forks e reorgs.
- Raiz vinculada ao chain ID, época, âncora e estado de todas as lanes.
- Reconstrução determinística de nó novo.
- Novos participantes recebem predecessor inicial canônico.
- A lane de quem sai é preservada para impedir reset/replay, mas tickets de um
  participante inativo são rejeitados.
- Persistência é somente cache; corrupção e encoding não canônico são
  rejeitados.

## Limites honestos

- O envelope ainda não faz parte do header LQC ativo.
- Ainda não existem RPC ou gossip de tickets.
- Ainda não existe regra de seleção por trabalho.
- Mudança de época/âncora será definida junto à ativação de laboratório.
- Inclusão anticensura com milhares de participantes continua pendente.
- A vulnerabilidade Sybil da seleção atual continua existindo até a integração
  e a repetição dos auditores ofensivos.

O genesis congelado permanece byte a byte idêntico.
