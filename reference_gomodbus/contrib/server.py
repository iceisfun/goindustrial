import asyncio
import logging
from pymodbus.server import StartAsyncTcpServer  # Use StartAsyncTcpServer instead
from pymodbus.datastore import ModbusServerContext, ModbusSlaveContext, ModbusSequentialDataBlock
from pymodbus.device import ModbusDeviceIdentification

# Configure logging
logging.basicConfig(format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO)

# Setup datastore with all 0s
store = ModbusSlaveContext(
    di=ModbusSequentialDataBlock(0, [0]*10000),
    co=ModbusSequentialDataBlock(0, [0]*10000),
    hr=ModbusSequentialDataBlock(0, [0]*10000),
    ir=ModbusSequentialDataBlock(0, [0]*10000)
)
context = ModbusServerContext(slaves=store, single=True)

identity = ModbusDeviceIdentification()
identity.VendorName = "tester"
identity.ProductCode = "TS"
identity.VendorUrl = "http://github.com"
identity.ProductName = "tester"
identity.ModelName = "pymodbus"
identity.MajorMinorRevision = "1.0"

async def run_server():
    print("Starting PyModbus server on 0.0.0.0:502")
    logging.info("Starting PyModbus server on 0.0.0.0:502")
    # Use StartAsyncTcpServer directly
    server = await StartAsyncTcpServer(
        context=context,
        identity=identity,
        address=("0.0.0.0", 502)
    )
    print("PyModbus server started successfully!")

    # This will keep the server running until interrupted
    await asyncio.Event().wait()

if __name__ == "__main__":
    asyncio.run(run_server())