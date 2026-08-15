# Rabbit Chain — auditoria de carga, mempool e rejeições

O executor `scripts/rabbit-devnet/run-transaction-stress-audit.sh` utiliza o
laboratório de 20 produtores já inicializado. Ele não reinicia a rede e não
modifica genesis ou consenso.

O cenário padrão envia 125 transferências EIP-1559 em cinco lotes. Cada lote é
submetido rapidamente com nonces consecutivos e aguardado até a confirmação
canônica antes do próximo lote.

São verificados todos os receipts, taxas, tips, burn, saldos, nonces e blocos de
inclusão. Todos os 20 nós precisam possuir os mesmos blocos e receipts de
amostra, e todos os txpools precisam terminar vazios.

Ao final também são submetidas quatro transações que devem ser rejeitadas:

1. repetição de uma transação já minerada;
2. nonce antigo;
3. valor superior ao saldo;
4. chain ID incorreto.

Depois da carga, o auditor profissional de recompensas percorre novamente toda
a cadeia para confirmar que transações e taxas não interferiram no Reward
Locker nem na emissão programada.
