# Auditoria das fronteiras monetárias da Rabbit Chain

O executor `scripts/rabbit-devnet/run-reward-boundary-audit.sh` valida as
alturas monetárias sem produzir milhões de blocos. Cada cenário cria um StateDB
isolado e chama diretamente as implementações reais de vesting e `Finalize`.

Cobertura:

- bloco 100.000 bloqueado e bloco 100.001 líquido;
- releases de 25%, 50%, 75% e 100% nos blocos 3.253.600, 4.042.000,
  4.830.400 e 5.618.800;
- bloco anterior, exato e posterior aos três halvings;
- paridade entre `consensus/lqc` e `consensus/lqcv2`;
- produtor 70%, committee 30% e fallback sem recompensa;
- conservação de remainder com committee de sete membros;
- idempotência, catch-up tardio e release global;
- emissão acumulada exata em wei;
- persistência do locker após finalização EIP-158;
- comparação entre tamanhos configurados e observados da seleção.

O auditor não modifica genesis, bancos de dados ou código de consenso.
