# Rabbit Work V1 — perfil de produção

O release Linux/amd64 da Rabbit mainnet deve ser compilado com:

```text
rabbit_workv1 rabbit_randomx
```

O build `rabbit_workv1_engine_lab` continua reservado a genesis isolado. O
binário default recusa o genesis oficial `RABBIT_MAINNET_GENESIS_V1`, evitando
que um operador crie acidentalmente uma cadeia incompatível sem Work V1.

No genesis oficial, o build de produção ativa automaticamente o transporte
`lqcw/1`; o flag `--lqc.worktickets.labtransport` continua proibido.

## Regras congeladas

- initial Work difficulty: `lqc.proofDifficulty` do genesis (`100000`);
- retarget canônico atrasado por paridade de epochs;
- elegibilidade na admissão: snapshot histórico do challenge block;
- seleção: seats ainda elegíveis no registry canônico atual;
- reward: 70% para o autor efetivo autorizado e 30% para o comitê por seat;
- fallback autorizado recebe a parcela de produtor quando efetivamente produz;
- sem WorkSeats elegíveis: registry preserva liveness e o subsídio base é zero;
- reconstrução de runtime e anchors segue `(hash, number)` da própria branch;
- pool pending é persistido e tickets removidos por reorg são readmitidos
  somente enquanto ainda válidos.

## Build oficial

Use `scripts/rabbit-release/build-rabbit-mainnet-workv1.sh`. O script:

- confere o genesis congelado;
- confere o commit e a biblioteca RandomX pinados;
- exige Linux/amd64 e Go 1.24.0;
- executa testes default e de produção;
- cria um binário com `-trimpath`;
- força `-Wl,-z,noexecstack` e rejeita `GNU_STACK RWE`;
- inicializa datadirs descartáveis com o genesis oficial;
- prova que o binário de produção ativa Work V1;
- prova que o binário default recusa a mainnet;
- grava `SHA256SUMS`.

Nunca use `geth --mainnet`: esse flag pertence à Ethereum mainnet upstream.
Rabbit usa `geth init networks/rabbit-mainnet/genesis.json` e `--networkid 928`.

O build fecha o release candidate do software. O lançamento público ainda
requer bootnodes/ENRs reais, servidores independentes, RPC, archive node e
explorer. Nenhum endpoint ou nodekey do laboratório pode ser reutilizado.
