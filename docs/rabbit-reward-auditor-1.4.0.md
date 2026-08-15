# Rabbit reward auditor 1.4.0

Esta versão corrige somente a ferramenta de auditoria. Nenhuma linha de
`consensus/lqc` e nenhum byte do genesis congelado fazem parte do pacote.

## Diagnóstico do relatório 20260811-183144

- Os testes de transação, queda parcial, retorno, parada total e reinício dos
  21 nós passaram.
- O auditor antigo ainda esperava que as recompensas dos blocos 1 a 100000
  fossem bloqueadas, embora a regra ativa seja crédito líquido imediato.
- O auditor antigo conhecia inicialmente apenas os 20 bootstraps. O node21 só
  era adicionado à lista observada depois de produzir seu primeiro bloco.
- Nos blocos 98 e 99, uma parcela de committee de `0,18 RAB` foi creditada ao
  node21 antes de sua primeira produção. As duas parcelas somam exatamente
  `0,36 RAB`, a diferença apresentada no relatório antigo.
- As diferenças por carteira também vieram da reconstrução do committee usando
  a lista bootstrap estática em vez do registry canônico.

## Correções

1. O registry é reconstruído desde `registryProtocolBlock` com os envelopes dos
   headers e `RegistrySnapshot.ApplyHeader`.
2. REGISTER, HEARTBEAT, EXIT, missed turns, jail, fila e `registryRoot` são
   validados em cada altura.
3. Um endereço registrado passa a ser observado no mesmo bloco do REGISTER,
   antes de qualquer pagamento de committee.
4. O committee usa `committeeSize` quando explícito ou a regra dinâmica de 10%
   com os limites `committeeMin`/`committeeMax`.
5. Toda recompensa de mineração é modelada como líquida imediatamente; o
   storage legado de vesting deve permanecer vazio e inalterado.

## Resultado esperado

No laboratório atual, a nova auditoria deve contar `1,20 RAB` por bloco, obter
diferença total de `0 wei` e reconstruir exatamente os destinatários do
produtor e do committee. Depois desse PASS, o auditor de assinaturas percorre
todos os blocos canônicos.

