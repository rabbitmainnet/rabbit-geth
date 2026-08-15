# Rabbit Chain — auditoria do cadastro permissionless

Esta auditoria verifica se nós independentes, partindo do mesmo head canônico,
calculam exatamente o mesmo conjunto de participantes.

Ela cobre três propriedades obrigatórias:

1. dois produtores com memórias locais diferentes precisam derivar a mesma fila;
2. um produtor fora do genesis precisa possuir um caminho determinístico de entrada;
3. um nó novo precisa reconstruir o cadastro somente a partir de dados canônicos.

O laboratório de 20 produtores usa `registryMode=bootstrap`. Por isso, os testes
de convergência, transações, resiliência e recompensas aprovados não demonstram
entrada permissionless.

O genesis mainnet usa `registryMode=native`. Na implementação auditada, o modo
native consulta `runtimeRegistry`, que existe apenas na memória de cada processo.
Enquanto isso não for substituído por dados determinísticos derivados da cadeia,
o lançamento público deve permanecer bloqueado.
