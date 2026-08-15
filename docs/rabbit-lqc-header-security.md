# Rabbit Chain — segurança básica do header LQC 1.0.1

Esta etapa fecha apenas as validações básicas ausentes no header do consenso
`consensus/lqc`. Ela não muda a economia da Rabbit Chain e não descongela a
mainnet.

## Regras adicionadas

- rejeição de bloco com mais de 30 segundos no futuro;
- rejeição segura de overflow no cálculo do slot do produtor/fallback;
- rejeição de número de bloco que não caiba em `uint64`;
- `gasLimit` limitado e vinculado ao `gasLimit` do bloco pai;
- `gasUsed` nunca pode superar `gasLimit`;
- `baseFee` obrigatória e calculada conforme EIP-1559 quando London está ativo;
- `baseFee` proibida antes de London;
- rejeição dos campos Shanghai, Cancun e Prague enquanto esses forks não
  estiverem implementados e auditados no LQC;
- tratamento do genesis como ponto inicial comprometido pelo hash, preservando
  o `extraData` congelado da Rabbit mainnet.

## O que não foi alterado

- recompensa imediata de mineração;
- divisão produtor/committee;
- halving e eras;
- fila e cadastro permissionless;
- LightHash;
- genesis da mainnet.

## Barreira ainda aberta

Os blocos ainda precisam receber uma assinatura criptográfica verificável do
produtor selecionado. Enquanto essa assinatura não estiver implementada,
testada em múltiplos nós e auditada, a Rabbit mainnet continua bloqueada para
lançamento.

## Validação

Execute somente:

```bash
cd "$HOME/projects/rabbit-geth" && chmod +x ./scripts/rabbit-devnet/validate-lqc-header-security.sh && ./scripts/rabbit-devnet/validate-lqc-header-security.sh
```

O resultado esperado desta etapa é `SEGURANÇA BÁSICA DO HEADER: PASS`, seguido
do aviso `MAINNET: NÃO LANÇAR AINDA`.

## Correção 1.0.1

A suíte ampliada encontrou dois testes antigos incompatíveis com regras que já
estavam no código:

- um fixture construía um header com `gasLimit` zero; ele agora herda o
  `gasLimit` do bloco pai, como um bloco válido deve fazer;
- o teste do RPC ainda esperava validade fixa de 64 blocos, embora o protocolo
  anuncie e use `MaxRegistryOperationLifetime = 256`; a expectativa agora é
  calculada diretamente do parâmetro canônico.

Essas duas mudanças são somente em testes. Nenhuma validação do header foi
removida ou enfraquecida.
