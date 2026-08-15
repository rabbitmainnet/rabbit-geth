# Rabbit Chain Transaction Auditor

Teste de integração somente para o laboratório Rabbit. Assina localmente uma transferência
EIP-1559 com o keystore criptografado do node20, envia a transação bruta por IPC e verifica:

- inclusão e receipt canônico;
- valor e nonce do remetente;
- saldo do destinatário;
- custo total do gas;
- base fee queimada;
- tip creditado ao producer do bloco;
- hash do bloco e receipt nos nós selecionados.

O delta da transação é isolado com `prestateTracer`. Dessa forma, recompensas
imediatas de produtor e committee creditadas no mesmo bloco não são confundidas
com valor, gas, burn ou tip da transferência.

Por padrão todos os 20 nós são verificados. A opção `--verify-nodes` permite
informar uma lista como `1,3,4,20` para auditar uma transação durante um teste
controlado no qual os demais nós estão desligados.

O script `scripts/rabbit-devnet/run-transaction-audit.sh` executa este programa e, em seguida,
o auditor de rewards para separar efeitos da transação, reward líquido e Reward Locker.

A conta node20 recebe 1.000 RAB somente no `genesis-runtime.json` temporário criado pelo script
do laboratório. `networks/rabbit-devnet/genesis.json` e o genesis de mainnet não são alterados.

O script `scripts/rabbit-devnet/run-network-resilience-audit.sh` usa essa opção
para testar transações com perda parcial de produtores, retorno dos produtores
e reinício completo dos 20 nós.
