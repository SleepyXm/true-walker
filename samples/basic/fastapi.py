import fastapi

app = fastapi.FastAPI()

@app.get("/users")
def users():
    pass